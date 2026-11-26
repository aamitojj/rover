package main

// Rover 0.5.0 tests: Jiv intents, confirm gating, the ask stream, the
// offline brain sidecar (against a mock llama-server started by re-running
// this test binary) and the hosting accounts portal. No real network calls.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------- mock llama-server (child process) ----------

func TestMain(m *testing.M) {
	if envOr("MOCK_LLAMA") != "" {
		mockLlama()
		return
	}
	os.Exit(m.Run())
}

func mockLlama() {
	var args []string
	_ = json.Unmarshal([]byte(envOr("MOCK_ARGS")), &args)
	port, key := "", ""
	for i := 0; i+1 < len(args); i++ {
		switch args[i] {
		case "--port":
			port = args[i+1]
		case "--api-key":
			key = args[i+1]
		}
	}
	if envOr("MOCK_LLAMA") == "crash" || port == "" {
		os.Exit(3)
	}
	if envOr("MOCK_LLAMA") == "gpucrash" && strings.Contains(strings.Join(args, " "), "--fit") {
		os.Exit(4) // "the graphics card could not load the model"
	}
	_ = os.WriteFile(envOr("MOCK_ARGFILE"), []byte(strings.Join(args, " ")), 0o600)
	start := time.Now()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if time.Since(start) < 300*time.Millisecond {
			w.WriteHeader(503) // "loading model"
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(401)
			return
		}
		var req struct {
			Messages []struct{ Role, Content string }
			Tools    []any
			Stream   *bool
			Format   map[string]any `json:"response_format"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		last := ""
		if n := len(req.Messages); n > 0 {
			last = req.Messages[n-1].Content
		}
		if req.Stream != nil && !*req.Stream && req.Format != nil { // brainJSON
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"steps":[{"tool":"check_services","server":"s2"}]}`}}}})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(v any) { b, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", b) }
		if strings.Contains(last, "MOCKTOOL") && len(req.Tools) > 0 {
			send(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"name": "redeploy_site", "arguments": `{"site":"shop"}`}}}}}}})
		} else {
			for _, p := range []string{"Hello ", "from the ", "offline brain."} {
				send(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": p}}}})
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	srv := &http.Server{Addr: "127.0.0.1:" + port, Handler: mux}
	_ = srv.ListenAndServe()
}

// installMockBrain creates fake brain files and points the sidecar at this
// test binary. mode "ok" or "crash".
func installMockBrain(t *testing.T, mode string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("ROVER_BRAIN_DIR", dir)
	m, _ := brainModelByID("qwen3-1.7b")
	os.WriteFile(filepath.Join(dir, m.File), []byte("GGUF fake"), 0o600)
	rt := filepath.Join(dir, "runtime-"+llamaBuild)
	os.MkdirAll(rt, 0o755)
	os.WriteFile(filepath.Join(rt, serverExeName()), []byte("x"), 0o755)
	argFile := filepath.Join(dir, "args.txt")
	old := brainCommand
	brainCommand = func(exe string, args []string) *exec.Cmd {
		b, _ := json.Marshal(args)
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), "ROVER_MOCK_LLAMA="+mode, "ROVER_MOCK_ARGS="+string(b), "ROVER_MOCK_ARGFILE="+argFile)
		return cmd
	}
	withConfig(func(c *Config) error {
		c.Jiv = &JivSettings{Model: "qwen3-1.7b", Lang: "auto", IdleMin: 20}
		return nil
	})
	brainSidecar.mu.Lock()
	brainSidecar.crashes = 0
	brainSidecar.mu.Unlock()
	t.Cleanup(func() { brainSidecar.Stop(); brainCommand = old })
	return argFile
}

func TestBrainSidecarMock(t *testing.T) {
	isolate(t)
	argFile := installMockBrain(t, "ok")
	c := snapshotConfig()
	if !brainAvailable(&c) {
		t.Fatal("brain should be available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	base, key, err := brainSidecar.Ensure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st := brainSidecar.Status(); st["state"] != "ready" {
		t.Fatalf("state %v", st)
	}
	a, _ := os.ReadFile(argFile)
	for _, want := range []string{"--host 127.0.0.1", "--api-key " + key, "-c 8192", "--no-webui", "--jinja"} {
		if !strings.Contains(string(a), want) {
			t.Errorf("llama-server args missing %q: %s", want, a)
		}
	}
	if strings.Contains(string(a), "0.0.0.0") {
		t.Error("brain must only listen on localhost")
	}
	// second Ensure reuses the running process
	base2, _, err := brainSidecar.Ensure(ctx)
	if err != nil || base2 != base {
		t.Fatal("not reused", base2, err)
	}
	// the ask stream uses it, answers and tool calls become cards
	withConfig(func(c *Config) error {
		c.Sites = append(c.Sites, &Site{Slug: "shop", Name: "Shop"})
		return nil
	})
	evs := askStream(t, map[string]any{"q": "tell me a fun fact about websites", "target": "brain"})
	if txt := evText(evs); !strings.Contains(txt, "Hello from the offline brain.") {
		t.Fatalf("brain text missing: %q %v", txt, evs)
	}
	evs = askStream(t, map[string]any{"q": "MOCKTOOL zap it", "target": "brain"})
	var card map[string]any
	for _, e := range evs {
		if e["t"] == "action" {
			card, _ = e["card"].(map[string]any)
		}
	}
	if card == nil || card["name"] != "redeploy_site" || card["safe"] != false || card["id"] == "" {
		t.Fatalf("tool call should become a confirm card: %v", evs)
	}
	// idle unload
	if !brainSidecar.idleCheck(time.Now().Add(2*time.Hour), 20*time.Minute) {
		t.Error("idle brain should unload")
	}
	if st := brainSidecar.Status(); st["state"] == "ready" {
		t.Error("still ready after idle unload")
	}
}

func TestBrainSidecarCrashFallsBack(t *testing.T) {
	isolate(t)
	installMockBrain(t, "crash")
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, _, err := brainSidecar.Ensure(ctx)
		cancel()
		if err == nil {
			t.Fatal("crashing brain must report an error")
		}
		time.Sleep(50 * time.Millisecond)
	}
	c := snapshotConfig()
	if brainAvailable(&c) {
		t.Error("after 3 crashes Jiv must stop using the brain")
	}
	// Jiv still answers from the built-in knowledge
	evs := askStream(t, map[string]any{"q": "how do I add my domain", "target": "auto"})
	if !strings.Contains(evText(evs), "domain") {
		t.Fatalf("no built-in answer: %v", evs)
	}
}

// ---------- ask stream ----------

func jivMux() *http.ServeMux {
	mux := http.NewServeMux()
	registerJivRoutes(mux)
	registerHostingRoutes(mux)
	registerServerRoutes(mux)
	return mux
}

func askStream(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	jivMux().ServeHTTP(rec, httptest.NewRequest("POST", "/api/jiv/ask", bytes.NewReader(b)))
	if rec.Code != 200 {
		t.Fatalf("ask: %d %s", rec.Code, rec.Body)
	}
	var out []map[string]any
	sc := bufio.NewScanner(rec.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e map[string]any
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	if len(out) == 0 || out[len(out)-1]["t"] != "done" {
		t.Fatalf("stream did not finish: %v", out)
	}
	return out
}

func evText(evs []map[string]any) string {
	var s strings.Builder
	for _, e := range evs {
		if e["t"] == "text" {
			s.WriteString(fmt.Sprint(e["v"]))
		}
	}
	return s.String()
}

func TestJivAskBuiltinOffline(t *testing.T) {
	isolate(t)
	t.Setenv("ROVER_BRAIN_DIR", t.TempDir()) // no brain installed
	withConfig(func(c *Config) error {
		c.Sites = append(c.Sites, &Site{Slug: "bakery", Name: "Bakery", Domain: "bakery.example", DomainLive: true})
		return nil
	})
	evs := askStream(t, map[string]any{"q": "how do i connect a godaddy domain", "target": "auto", "screen": "domains"})
	if evs[0]["t"] != "start" || evs[0]["mode"] != "builtin" {
		t.Fatalf("offline answer must say it is built-in: %v", evs[0])
	}
	if !strings.Contains(strings.ToLower(evText(evs)), "godaddy") {
		t.Fatalf("expected the GoDaddy article: %v", evs)
	}
	var src bool
	for _, e := range evs {
		if e["t"] == "sources" {
			src = true
		}
	}
	if !src {
		t.Error("answer should list its source article")
	}
	// Hindi question -> Hindi answer
	evs = askStream(t, map[string]any{"q": "डोमेन कैसे जोड़ें", "target": "builtin"})
	if !strings.ContainsAny(evText(evs), "डोकैसे") {
		t.Errorf("expected Hindi text: %q", evText(evs))
	}
	// a request becomes a card, never a direct change
	evs = askStream(t, map[string]any{"q": "roll back bakery", "target": "builtin"})
	found := false
	for _, e := range evs {
		if e["t"] == "action" {
			c := e["card"].(map[string]any)
			found = c["name"] == "rollback_site" && c["safe"] == false
		}
	}
	if !found {
		t.Fatalf("expected a rollback confirm card: %v", evs)
	}
	// nonsense: still a polite answer, with topics to pick from
	evs = askStream(t, map[string]any{"q": "qwxz plorb", "target": "builtin"})
	if evText(evs) == "" {
		t.Error("empty answer for unknown question")
	}
}

func TestJivCloudNeedsConsent(t *testing.T) {
	isolate(t)
	t.Setenv("ROVER_BRAIN_DIR", t.TempDir())
	withConfig(func(c *Config) error {
		c.AIConns = map[string]*AIConn{"openai": {Key: "plain:sk-test-123"}}
		return nil
	})
	evs := askStream(t, map[string]any{"q": "hello", "target": "openai", "model": "gpt-x"})
	var ce map[string]any
	for _, e := range evs {
		if e["t"] == "error" {
			ce = e
		}
	}
	if ce == nil || ce["kind"] != "consent" || ce["conn"] != "openai" {
		t.Fatalf("cloud without consent must stop: %v", evs)
	}
}

// ---------- intents ----------

func TestDetectIntent(t *testing.T) {
	c := &Config{Sites: []*Site{{Slug: "bakery", Name: "Bakery", Domain: "bakery.in"}, {Slug: "portfolio", Name: "My Portfolio"}}}
	cases := []struct {
		q, tool, site string
		question      bool
	}{
		{"roll back bakery", "rollback_site", "bakery", false},
		{"please redeploy portfolio", "redeploy_site", "portfolio", false},
		{"why is bakery down?", "diagnose", "bakery", true},
		{"bakery.in is not working", "check_dns", "bakery", false},
		{"is everything ok?", "diagnose", "", true},
		{"check all my sites", "run_health", "", false},
		{"explain the logs of portfolio", "explain_logs", "portfolio", false},
		{"open hosting accounts", "open_page", "", false},
		{"connect my hetzner account", "add_hosting_account", "", false},
		{"download the offline brain", "install_brain", "", false},
		{"back up everything now", "backup_now", "", false},
		{"turn off auto deploy for bakery", "toggle_auto_deploy", "bakery", false},
		{"bakery ka purana version lagao", "rollback_site", "bakery", false},
		{"how do I add a domain?", "", "", true},
	}
	for _, tc := range cases {
		in := detectIntent(c, tc.q)
		got, site := "", ""
		if in.Call != nil {
			got = in.Call.Name
			site, _ = in.Call.Args["site"].(string)
		}
		if got != tc.tool || (tc.site != "" && site != tc.site) {
			t.Errorf("%q: got %s(%s), want %s(%s)", tc.q, got, site, tc.tool, tc.site)
		}
	}
	if in := detectIntent(c, "connect my hetzner account"); in.Call == nil || in.Call.Args["provider"] != "hetzner" {
		t.Error("provider not picked up")
	}
	if in := detectIntent(c, "open hosting accounts"); in.Call == nil || in.Call.Args["page"] != "hosting" {
		t.Errorf("page: %+v", in.Call)
	}
}

// ---------- confirm gating ----------

func TestActionConfirmGating(t *testing.T) {
	isolate(t)
	withConfig(func(c *Config) error {
		c.Sites = append(c.Sites, &Site{Slug: "shop", Name: "Shop"})
		return nil
	})
	var hits []string
	old := appInner
	appInner = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.Method+" "+r.URL.Path)
		w.Write([]byte(`{"ok":true}`))
	})
	t.Cleanup(func() { appInner = old })
	mux := jivMux()

	if _, err := proposeAction(ToolCall{Name: "rm_rf_server", Args: map[string]any{}}); err == nil {
		t.Fatal("unknown tool must be rejected")
	}
	if _, err := proposeAction(ToolCall{Name: "redeploy_site", Args: map[string]any{"site": "nope"}}); err == nil {
		t.Fatal("unknown project must be rejected")
	}
	code, card := call(t, mux, "POST", "/api/jiv/propose", map[string]any{"name": "redeploy_site", "args": map[string]any{"site": "Shop"}})
	if code != 200 || card["safe"] != false || card["id"] == "" {
		t.Fatalf("propose: %d %v", code, card)
	}
	if card["args"].(map[string]any)["site"] != "shop" {
		t.Error("site name should resolve to the slug")
	}
	id := card["id"].(string)
	code, out := call(t, mux, "POST", "/api/jiv/actions/"+id+"/run", map[string]any{})
	if code != 428 || len(hits) != 0 {
		t.Fatalf("unconfirmed run must not act: %d %v %v", code, out, hits)
	}
	code, out = call(t, mux, "POST", "/api/jiv/actions/"+id+"/run", map[string]any{"confirmed": true})
	if code != 200 || len(hits) == 0 {
		t.Fatalf("confirmed run: %d %v %v", code, out, hits)
	}
	n := len(hits)
	code, _ = call(t, mux, "POST", "/api/jiv/actions/"+id+"/run", map[string]any{"confirmed": true})
	if code != 409 || len(hits) != n {
		t.Fatalf("an action must run only once: %d", code)
	}
	code, _ = call(t, mux, "POST", "/api/jiv/actions/forged-id/run", map[string]any{"confirmed": true})
	if code != 404 {
		t.Fatalf("made-up ids must fail: %d", code)
	}
	// expired cards are dropped
	_, card = call(t, mux, "POST", "/api/jiv/propose", map[string]any{"name": "redeploy_site", "args": map[string]any{"site": "shop"}})
	pendMu.Lock()
	pending[card["id"].(string)].created = time.Now().Add(-2 * time.Hour)
	pendMu.Unlock()
	call(t, mux, "POST", "/api/jiv/propose", map[string]any{"name": "open_page", "args": map[string]any{"page": "hosting"}}) // triggers cleanup
	if code, _ := call(t, mux, "POST", "/api/jiv/actions/"+card["id"].(string)+"/run", map[string]any{"confirmed": true}); code != 404 {
		t.Fatalf("expired action ran: %d", code)
	}
	// safe navigation runs without confirm and changes nothing server-side
	_, nav := call(t, mux, "POST", "/api/jiv/propose", map[string]any{"name": "open_page", "args": map[string]any{"page": "hosting"}})
	if nav["safe"] != true || nav["client"] != true {
		t.Fatalf("open_page: %v", nav)
	}
	code, out = call(t, mux, "POST", "/api/jiv/actions/"+nav["id"].(string)+"/run", map[string]any{})
	if code != 200 || out["client"] != true || len(hits) != n {
		t.Fatalf("safe client action: %d %v", code, out)
	}
	// every state-changing tool is marked unsafe
	for _, a := range jivActions {
		if a.Safe && !a.Client {
			switch a.Def.Name {
			case "diagnose", "check_dns", "run_health", "explain_logs", "open_project_folder": // read-only / opens Explorer on this PC
			default:
				t.Errorf("%s is marked safe but runs on the server", a.Def.Name)
			}
		}
	}
}

// ---------- hosting accounts ----------

func TestHostingAccountVultrFake(t *testing.T) {
	isolate(t)
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/instances":
			w.Write([]byte(`{"instances":[{"id":"i-1","label":"web-1","main_ip":"203.0.113.5","status":"active","power_status":"running","plan":"vc2-2c-4gb","region":"fra","vcpu_count":2,"ram":4096,"disk":80}]}`))
		case "/account":
			w.Write([]byte(`{"account":{"email":"owner@example.com","name":"Owner"}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	old := hostBase["vultr"]
	hostBase["vultr"] = srv.URL
	t.Cleanup(func() { hostBase["vultr"] = old })
	mux := jivMux()
	const tok = "VULTRTOKEN1234567890ABCDEFGHIJKLMNOPQR"
	code, out := call(t, mux, "POST", "/api/hosting/accounts", map[string]any{"provider": "vultr", "nickname": "Client A", "fields": map[string]string{"token": tok}})
	if code != 200 {
		t.Fatalf("add: %d %v", code, out)
	}
	if auth != "Bearer "+tok {
		t.Error("token not sent as bearer")
	}
	// a second account at the same provider is fine
	code, _ = call(t, mux, "POST", "/api/hosting/accounts", map[string]any{"provider": "vultr", "nickname": "Client B", "fields": map[string]string{"token": tok + "X"}})
	if code != 200 {
		t.Fatal("second account", code)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/hosting/accounts", nil))
	if strings.Contains(rec.Body.String(), tok) {
		t.Fatal("token leaked to the UI")
	}
	var lst struct {
		Accounts []struct {
			ID, Nickname string
			Servers      []HostedServer
		}
	}
	json.Unmarshal(rec.Body.Bytes(), &lst)
	if len(lst.Accounts) != 2 || len(lst.Accounts[0].Servers) != 1 || lst.Accounts[0].Servers[0].IP != "203.0.113.5" {
		t.Fatalf("accounts: %s", rec.Body)
	}
	// Jiv's context names accounts but never tokens
	c := snapshotConfig()
	if hc := hostingContext(&c); !strings.Contains(hc, "Client A") || strings.Contains(hc, tok) || strings.Contains(hc, "203.0.113.5") {
		t.Fatalf("hosting context: %s", hc)
	}
	// creating a server needs confirmed=true
	id := lst.Accounts[0].ID
	code, out = call(t, mux, "POST", "/api/hosting/accounts/"+id+"/create", map[string]any{"plan": "vc2-2c-4gb", "region": "fra"})
	if code != 428 {
		t.Fatalf("create without confirm: %d %v", code, out)
	}
	// remove
	code, _ = call(t, mux, "DELETE", "/api/hosting/accounts/"+id, nil)
	if code != 200 || len(snapshotConfig().HostingAccounts) != 1 {
		t.Fatal("remove failed")
	}
	// bad token -> friendly error, nothing saved
	hostBase["vultr"] = srv.URL + "/nope"
	code, out = call(t, mux, "POST", "/api/hosting/accounts", map[string]any{"provider": "vultr", "fields": map[string]string{"token": tok}})
	if code == 200 || len(snapshotConfig().HostingAccounts) != 1 {
		t.Fatalf("bad token accepted: %d %v", code, out)
	}
}

func TestSigV4Vector(t *testing.T) {
	// AWS SigV4 test suite "get-vanilla".
	u, _ := url.Parse("https://example.amazonaws.com/")
	hdr := map[string]string{}
	sigV4(hdr, "GET", u, nil, "AKIDEXAMPLE", "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", "us-east-1", "service", time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, Signature=5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"
	if hdr["Authorization"] != want {
		t.Fatalf("got %s", hdr["Authorization"])
	}
}

func TestHostingProvidersList(t *testing.T) {
	isolate(t)
	rec := httptest.NewRecorder()
	jivMux().ServeHTTP(rec, httptest.NewRequest("GET", "/api/hosting/providers", nil))
	var out struct {
		Providers []struct {
			ID, Auth  string
			CanCreate bool `json:"can_create"`
		}
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	got := map[string]string{}
	for _, p := range out.Providers {
		got[p.ID] = fmt.Sprintf("%s/%v", p.Auth, p.CanCreate)
	}
	for id, want := range map[string]string{"digitalocean": "oauth/true", "hetzner": "token/true", "vultr": "token/true", "linode": "token/true", "lightsail": "keys/true", "oracle": "keys/false", "other": "ssh/false"} {
		if got[id] != want {
			t.Errorf("%s: %s, want %s", id, got[id], want)
		}
	}
	if len(out.Providers) < 11 {
		t.Errorf("only %d providers", len(out.Providers))
	}
}

func TestBrainVerifiedDownload(t *testing.T) {
	data := bytes.Repeat([]byte("dilreet"), 50000)
	sh := sha256.Sum256(data)
	sum := hex.EncodeToString(sh[:])
	j := &Job{}
	var fail = true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail && r.Header.Get("Range") == "" { // drop the first transfer half-way
			fail = false
			w.Header().Set("Content-Length", fmt.Sprint(len(data)))
			w.Write(data[:len(data)/2])
			if hj, ok := w.(http.Hijacker); ok {
				c, _, _ := hj.Hijack()
				c.Close()
			}
			return
		}
		http.ServeContent(w, r, "m.gguf", time.Now(), bytes.NewReader(data))
	}))
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), "m.gguf")
	if err := fetchVerified(context.Background(), j, "Model", srv.URL, dst, sum, int64(len(data)), 0, 100); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); !bytes.Equal(b, data) {
		t.Fatal("content differs")
	}
	if err := fetchVerified(context.Background(), j, "Model", srv.URL, dst+"2", strings.Repeat("0", 64), int64(len(data)), 0, 100); err == nil {
		t.Fatal("wrong checksum must fail")
	}
	if _, err := os.Stat(dst + "2"); err == nil {
		t.Fatal("bad file must not be kept")
	}
}

func TestMigrateV04ToV05Servers(t *testing.T) {
	isolate(t)
	dir := envOr("CONFIG_DIR")
	old := `{"config_version":4,"server_ip":"198.51.100.7","ssh_port":22,"ssh_user":"root","coolify_url":"http://198.51.100.7:8000","coolify_token":"plain:tok-A","sites":[{"slug":"shop","name":"Shop"},{"slug":"blog","name":"Blog"}]}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadConfig(); err != nil {
		t.Fatal(err)
	}
	c := snapshotConfig()
	if c.ConfigVersion != currentConfigVersion || len(c.Servers) != 1 || c.ActiveServer != c.Servers[0].ID {
		t.Fatalf("servers not migrated: v%d %+v", c.ConfigVersion, c.Servers)
	}
	first := c.ActiveServer
	for _, s := range c.Sites {
		if s.Server != first {
			t.Fatalf("%s not assigned to the first server", s.Slug)
		}
	}
	if c.Jiv == nil || c.Jiv.IdleMin != 20 {
		t.Fatal("Jiv defaults missing")
	}
	// a second server: the old projects stay with the first one
	withConfig(func(c *Config) error { c.ServerIP = "203.0.113.20"; return nil })
	c = snapshotConfig()
	if len(c.Servers) != 2 || c.ActiveServer == first || c.CoolifyToken != "" {
		t.Fatalf("second server: %+v token=%q", c.Servers, c.CoolifyToken)
	}
	gate := serverGate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	code, out := call(t, gate, "POST", "/api/sites/shop/deploy", map[string]any{})
	if code != 409 || out["switch_server"] != first {
		t.Fatalf("gate: %d %v", code, out)
	}
	if code, _ := call(t, gate, "GET", "/api/sites", nil); code != 200 {
		t.Fatal("list must not be gated")
	}
	if err := activateServer(first); err != nil {
		t.Fatal(err)
	}
	c = snapshotConfig()
	if c.ServerIP != "198.51.100.7" || secret(c.CoolifyToken) != "tok-A" {
		t.Fatalf("switching back lost the first server's login: %s %q", c.ServerIP, c.CoolifyToken)
	}
	if code, _ := call(t, gate, "POST", "/api/sites/shop/deploy", map[string]any{}); code != 200 {
		t.Fatal("gate should open on the right server")
	}
}
