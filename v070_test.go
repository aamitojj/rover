package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- helpers ----------

// bashRunner runs scripts locally with bash; uploads are file copies.
type bashRunner struct {
	failUpload bool
	failRun    bool
	env        []string
	mu         sync.Mutex
	scripts    []string
}

func (b *bashRunner) Upload(local, remote string, progress func(done, total int64)) error {
	if b.failUpload {
		return errors.New("connection refused")
	}
	data, err := os.ReadFile(local)
	if err != nil {
		return err
	}
	if progress != nil {
		progress(int64(len(data)), int64(len(data)))
	}
	return os.WriteFile(remote, data, 0o600)
}

func (b *bashRunner) Run(ctx context.Context, script string, out io.Writer) error {
	b.mu.Lock()
	b.scripts = append(b.scripts, script)
	b.mu.Unlock()
	if b.failRun {
		return errors.New("Process exited with status 1")
	}
	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	cmd.Env = append(os.Environ(), b.env...)
	cmd.Stdout, cmd.Stderr = out, out
	return cmd.Run()
}

func (b *bashRunner) User() string { return "test" }

func tarGz(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	p := filepath.Join(t.TempDir(), "site.tar.gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

type multiFixture struct {
	roots   map[string]string
	runners map[string]*bashRunner
}

// multiSetup: project "shop" on servers a (main), b and c, each a temp dir.
func multiSetup(t *testing.T) *multiFixture {
	t.Helper()
	isolate(t)
	f := &multiFixture{roots: map[string]string{}, runners: map[string]*bashRunner{}}
	_ = withConfig(func(c *Config) error {
		for _, id := range []string{"a", "b", "c"} {
			c.Servers = append(c.Servers, &ServerProfile{ID: id, Label: "Server " + strings.ToUpper(id), ServerIP: "203.0.113." + map[string]string{"a": "1", "b": "2", "c": "3"}[id]})
		}
		c.ActiveServer = "a"
		c.Sites = append(c.Sites, &Site{Slug: "shop", Name: "Shop", Server: "a", Servers: []string{"b", "c"}, HAMode: "backup", AppUUID: "app-a"})
		return nil
	})
	for _, id := range []string{"a", "b", "c"} {
		f.roots[id] = t.TempDir()
		f.runners[id] = &bashRunner{}
	}
	old := openTarget
	openTarget = func(c *Config, s *Site, id string) (*serverTarget, error) {
		r := f.runners[id]
		if r == nil {
			return nil, errors.New("unknown")
		}
		return &serverTarget{ID: id, Label: serverLabel(c, id), r: r, root: f.roots[id], app: replicaApp(s, id)}, nil
	}
	t.Cleanup(func() { openTarget = old })
	return f
}

func liveOn(t *testing.T, root string) string {
	t.Helper()
	l, err := os.Readlink(filepath.Join(root, "shop", "html"))
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(l, "releases/")
}

// ---------- multi-server deploy ----------

func TestDeployToAllServers(t *testing.T) {
	f := multiSetup(t)
	tb := tarGz(t, map[string]string{"index.html": "<h1>v1</h1>"})
	j := newJob("test", func(*Job) error { return nil })
	if err := deployToAll(j, "shop", tb, 1, 11, defaultNginxConf()); err != nil {
		t.Fatalf("deploy: %v\n%s", err, strings.Join(j.Lines(200), "\n"))
	}
	c := snapshotConfig()
	s := findSite(&c, "shop")
	if s.Release == "" {
		t.Fatal("release not recorded")
	}
	for _, id := range []string{"a", "b", "c"} {
		if got := liveOn(t, f.roots[id]); got != s.Release {
			t.Errorf("server %s live=%q want %q", id, got, s.Release)
		}
		if r := s.Replicas[id]; r == nil || r.Status != "live" || r.Release != s.Release {
			t.Errorf("replica %s = %+v", id, r)
		}
		b, _ := os.ReadFile(filepath.Join(f.roots[id], "shop", "html", "index.html"))
		if string(b) != "<h1>v1</h1>" {
			t.Errorf("server %s serves %q", id, b)
		}
	}
}

func TestDeployPartialFailureAndRetry(t *testing.T) {
	f := multiSetup(t)
	f.runners["b"].failUpload = true
	tb := tarGz(t, map[string]string{"index.html": "v2"})
	j := newJob("test", func(*Job) error { return nil })
	err := deployToAll(j, "shop", tb, 1, 2, defaultNginxConf())
	var pe *partialError
	if !errors.As(err, &pe) || pe.Nothing {
		t.Fatalf("want partial error, got %v", err)
	}
	if len(pe.Failed) != 1 || pe.Failed[0] != "b" || len(pe.Live) != 2 {
		t.Fatalf("partial = %+v", pe)
	}
	if !strings.Contains(err.Error(), "Server B") || !strings.Contains(err.Error(), "Retry") {
		t.Errorf("message not helpful: %v", err)
	}
	c := snapshotConfig()
	s := findSite(&c, "shop")
	rel := s.Release
	if liveOn(t, f.roots["a"]) != rel || liveOn(t, f.roots["c"]) != rel || liveOn(t, f.roots["b"]) != "" {
		t.Fatalf("live: a=%s b=%s c=%s rel=%s", liveOn(t, f.roots["a"]), liveOn(t, f.roots["b"]), liveOn(t, f.roots["c"]), rel)
	}
	if s.Replicas["b"].Status != "failed" || !strings.Contains(s.Replicas["b"].Error, "refused") {
		t.Errorf("b replica = %+v", s.Replicas["b"])
	}
	// finishDeploy records "partial" with the release.
	views := replicaViews(&c, s, false)
	if views[1].Status != "failed" || views[0].Role != "main" {
		t.Errorf("views = %+v", views)
	}

	// Server B is back: Retry publishes the same release name there only.
	f.runners["b"].failUpload = false
	j2 := newJob("test", func(*Job) error { return nil })
	if err := deployToTargets(j2, &c, s, []string{"a", "b"}, tb, 1, 2, defaultNginxConf(), rel, true); err != nil {
		t.Fatalf("retry: %v\n%s", err, strings.Join(j2.Lines(100), "\n"))
	}
	if liveOn(t, f.roots["b"]) != rel {
		t.Errorf("b not in sync after retry")
	}
	if !strings.Contains(strings.Join(j2.Lines(100), "\n"), "already up to date") {
		t.Errorf("server a should have been left alone:\n%s", strings.Join(j2.Lines(100), "\n"))
	}
	c = snapshotConfig()
	if r := findSite(&c, "shop").Replicas["b"]; r.Status != "live" || r.Release != rel {
		t.Errorf("b after retry = %+v", r)
	}

	// Sync check reads every server.
	old := liveReleaseOn
	liveReleaseOn = func(c *Config, s *Site, id string) (string, error) {
		l, _ := os.Readlink(filepath.Join(f.roots[id], "shop", "html"))
		return strings.TrimPrefix(l, "releases/"), nil
	}
	defer func() { liveReleaseOn = old }()
	for _, v := range replicaViews(&c, findSite(&c, "shop"), true) {
		if v.InSync == nil || !*v.InSync {
			t.Errorf("%s not in sync: %+v", v.ID, v)
		}
	}
}

func TestDeployAllFailNothingChanged(t *testing.T) {
	f := multiSetup(t)
	for _, r := range f.runners {
		r.failUpload = true
	}
	tb := tarGz(t, map[string]string{"index.html": "x"})
	err := deployToAll(newJob("t", func(*Job) error { return nil }), "shop", tb, 1, 1, defaultNginxConf())
	var pe *partialError
	if !errors.As(err, &pe) || !pe.Nothing || !strings.Contains(err.Error(), "Nothing was changed") {
		t.Fatalf("got %v", err)
	}
	c := snapshotConfig()
	if findSite(&c, "shop").Release != "" {
		t.Error("release must not change when every server failed")
	}
}

func TestRollbackOnAllServers(t *testing.T) {
	f := multiSetup(t)
	j := newJob("t", func(*Job) error { return nil })
	_ = deployToAll(j, "shop", tarGz(t, map[string]string{"index.html": "one"}), 1, 3, defaultNginxConf())
	c := snapshotConfig()
	first := findSite(&c, "shop").Release
	time.Sleep(1100 * time.Millisecond)
	_ = deployToAll(j, "shop", tarGz(t, map[string]string{"index.html": "two"}), 1, 3, defaultNginxConf())
	if err := activateRelease("shop", first); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	for id, root := range f.roots {
		if liveOn(t, root) != first {
			t.Errorf("%s not rolled back", id)
		}
	}
}

func TestPartialStatusRecorded(t *testing.T) {
	isolate(t)
	pe := &partialError{Live: []string{"A"}, Failed: []string{"b"}, Labels: map[string]string{"b": "B"}, Reasons: map[string]string{"b": "down"}}
	var target *partialError
	if !errors.As(fmt.Errorf("wrap: %w", pe), &target) {
		t.Fatal("errors.As")
	}
}

// ---------- lite (plain nginx) servers ----------

func TestLiteConfTransform(t *testing.T) {
	s := &Site{Slug: "my-shop", Domain: "shop.example", IncludeWWW: true, Access: nil}
	conf := liteConf(siteConfExtras(defaultNginxConf(), s), "my-shop", "/data/hostpanel/sites/my-shop", liteHosts(s, &ServerProfile{ServerIP: "192.168.1.50"}))
	for _, want := range []string{"listen 443 ssl;", "server_name shop.example www.shop.example my-shop.192.168.1.50.sslip.io my-shop.local;", "root /data/hostpanel/sites/my-shop/html;", "$hpmy_shop_rw"} {
		if !strings.Contains(conf, want) {
			t.Errorf("missing %q in\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "default_server") || strings.Contains(conf, "$hp_") || strings.Contains(conf, "/usr/share/nginx") {
		t.Errorf("container leftovers in\n%s", conf)
	}
}

func TestLiteDeployWithFakeNginx(t *testing.T) {
	f := multiSetup(t)
	bin := t.TempDir()
	confd := t.TempDir()
	_ = os.WriteFile(filepath.Join(bin, "nginx"), []byte("#!/bin/sh\necho \"nginx $*\" >> "+filepath.Join(confd, "calls")+"\nexit 0\n"), 0o755)
	f.runners["b"].env = []string{"PATH=" + bin + ":" + os.Getenv("PATH"), "HP_NGINX_CONFD=" + confd}
	old := openTarget
	openTarget = func(c *Config, s *Site, id string) (*serverTarget, error) {
		t2, err := old(c, s, id)
		if id == "b" && err == nil {
			t2.lite, t2.hosts = true, []string{"shop.local"}
		}
		return t2, err
	}
	defer func() { openTarget = old }()
	j := newJob("t", func(*Job) error { return nil })
	if err := deployToAll(j, "shop", tarGz(t, map[string]string{"index.html": "lite"}), 1, 4, defaultNginxConf()); err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(j.Lines(200), "\n"))
	}
	inc, err := os.ReadFile(filepath.Join(confd, "dilreet-shop.conf"))
	if err != nil || !strings.Contains(string(inc), "include "+f.roots["b"]+"/shop/nginx/default.conf") {
		t.Fatalf("include file: %v %q", err, inc)
	}
	live, _ := os.ReadFile(filepath.Join(f.roots["b"], "shop", "nginx", "default.conf"))
	if !strings.Contains(string(live), "server_name shop.local;") || !strings.Contains(string(live), "root "+f.roots["b"]+"/shop/html;") {
		t.Errorf("lite config:\n%s", live)
	}
	calls, _ := os.ReadFile(filepath.Join(confd, "calls"))
	if !strings.Contains(string(calls), "nginx -t") || !strings.Contains(string(calls), "nginx -s reload") {
		t.Errorf("nginx calls: %q", calls)
	}
}

func TestHomeScriptsDryRun(t *testing.T) {
	cmd := exec.Command("bash", "-c", liteEngineScript)
	cmd.Env = append(os.Environ(), "HP_DRY=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{"+ env DEBIAN_FRONTEND=noninteractive apt-get install -y -q nginx ssl-cert", "+ systemctl enable --now nginx", "+ mkdir -p /data/hostpanel/sites"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	script := cloudflaredInstallScript("dilreet-tunnel-abcd1234", "/tmp/nothing")
	cmd = exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(), "HP_DRY=1")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "releases/download/"+cloudflaredVersion+"/cloudflared-linux-") || !strings.Contains(s, "sha256sum -c") || !strings.Contains(s, "systemctl enable --now dilreet-tunnel-abcd1234") {
		t.Errorf("dry run:\n%s", s)
	}
	if strings.Contains(script, "--token") {
		t.Error("the token must never be on a command line")
	}
	for _, a := range []string{"linux-amd64", "linux-arm64", "linux-armhf", "linux-arm"} {
		if !strings.Contains(script, cloudflaredSHA256[a]) {
			t.Errorf("checksum for %s missing", a)
		}
	}
}

func TestChooseEngine(t *testing.T) {
	cases := []struct {
		f    ServerFacts
		want string
	}{
		{ServerFacts{Arch: "aarch64", RAMMB: 8000, Supported: true}, "coolify"},
		{ServerFacts{Arch: "aarch64", RAMMB: 1000, Supported: true}, "lite"},
		{ServerFacts{Arch: "armv7l", RAMMB: 4000, Supported: true}, "lite"},
		{ServerFacts{Arch: "x86_64", RAMMB: 4000, OSName: "Arch Linux"}, "lite"},
	}
	for _, c := range cases {
		if got, _ := chooseEngine(c.f); got != c.want {
			t.Errorf("%+v -> %s want %s", c.f, got, c.want)
		}
	}
	if cloudflaredAsset("aarch64") != "linux-arm64" || cloudflaredAsset("armv7l") != "linux-armhf" || cloudflaredAsset("x86_64") != "linux-amd64" {
		t.Error("asset mapping")
	}
	if !isPrivateHost("192.168.1.5") || !isPrivateHost("raspberrypi.local") || !isPrivateHost("100.70.1.1") || isPrivateHost("203.0.113.9") {
		t.Error("isPrivateHost")
	}
}

// ---------- tunnels ----------

func TestTunnelIngress(t *testing.T) {
	ing := tunnelIngress("https://localhost:443", []string{"www.shop.example", "shop.example", "shop.example"})
	b, _ := json.Marshal(ing)
	want := `[{"hostname":"shop.example","service":"https://localhost:443","originRequest":{"noTLSVerify":true,"originServerName":"shop.example","httpHostHeader":"shop.example"}},{"hostname":"www.shop.example","service":"https://localhost:443","originRequest":{"noTLSVerify":true,"originServerName":"www.shop.example","httpHostHeader":"www.shop.example"}},{"service":"http_status:404"}]`
	if string(b) != want {
		t.Errorf("ingress\n got %s\nwant %s", b, want)
	}
	local := tunnelIngress("http://localhost:8080", []string{"a.example"})
	if local[0].OriginRequest.NoTLSVerify || local[0].OriginRequest.HTTPHostHeader != "a.example" || local[1].Service != "http_status:404" {
		t.Errorf("local ingress %+v", local)
	}
	if len(tunnelIngress("x", nil)) != 1 {
		t.Error("catch-all only")
	}
}

// mockCF records DNS and tunnel calls.
type mockCF struct {
	mu      sync.Mutex
	records map[string]*CFRecord // id -> record
	calls   []string
	config  json.RawMessage
	seq     int
}

func newMockCF(t *testing.T) *mockCF {
	m := &mockCF{records: map[string]*CFRecord{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.calls = append(m.calls, r.Method+" "+r.URL.Path)
		ok := func(v any) { b, _ := json.Marshal(map[string]any{"success": true, "result": v}); w.Write(b) }
		p := r.URL.Path
		switch {
		case p == "/accounts":
			ok([]map[string]string{{"id": "acc1", "name": "Me"}})
		case strings.HasSuffix(p, "/cfd_tunnel") && r.Method == "POST":
			ok(map[string]string{"id": "tun-1234567890", "name": "x", "token": "secret-token"})
		case strings.HasSuffix(p, "/configurations") && r.Method == "PUT":
			m.config, _ = io.ReadAll(r.Body)
			ok(map[string]any{})
		case strings.HasSuffix(p, "/dns_records") && r.Method == "GET":
			var out []CFRecord
			for _, rec := range m.records {
				if rec.Name == r.URL.Query().Get("name") {
					out = append(out, *rec)
				}
			}
			ok(out)
		case strings.HasSuffix(p, "/dns_records") && r.Method == "POST":
			var rec CFRecord
			_ = json.NewDecoder(r.Body).Decode(&rec)
			m.seq++
			rec.ID = fmt.Sprintf("n%d", m.seq)
			m.records[rec.ID] = &rec
			ok(rec)
		case strings.Contains(p, "/dns_records/") && r.Method == "PATCH":
			id := p[strings.LastIndex(p, "/")+1:]
			var f map[string]any
			_ = json.NewDecoder(r.Body).Decode(&f)
			rec := m.records[id]
			if c, ok := f["content"].(string); ok {
				rec.Content = c
			}
			if pr, ok := f["proxied"].(bool); ok {
				rec.Proxied = pr
			}
			ok(rec)
		case strings.Contains(p, "/dns_records/") && r.Method == "DELETE":
			delete(m.records, p[strings.LastIndex(p, "/")+1:])
			ok(map[string]any{})
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"success":false,"errors":[{"code":1,"message":"not found"}]}`))
		}
	}))
	old := cfBase
	cfBase = srv.URL
	t.Cleanup(func() { cfBase = old; srv.Close() })
	return m
}

func (m *mockCF) content(name string) (string, string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.records {
		if r.Name == name {
			return r.Type, r.Content, r.Proxied
		}
	}
	return "", "", false
}

func TestProjectTunnelTogether(t *testing.T) {
	isolate(t)
	m := newMockCF(t)
	tok, _ := protectString("cf-token")
	_ = withConfig(func(c *Config) error {
		c.CloudflareToken = tok
		c.Servers = []*ServerProfile{{ID: "a", Label: "A"}, {ID: "b", Label: "B", Kind: "home"}}
		c.Sites = []*Site{{Slug: "shop", Name: "Shop", Server: "a", Servers: []string{"b"}, HAMode: "together", Domain: "shop.example", CFZoneID: "z1"}}
		return nil
	})
	m.records["old"] = &CFRecord{ID: "old", Type: "A", Name: "shop.example", Content: "203.0.113.1"}
	var started []string
	old := installConnector
	installConnector = func(j *Job, c *Config, p *ServerProfile, id, token string) error {
		if token != "secret-token" {
			t.Errorf("token %q", token)
		}
		started = append(started, p.ID+":"+id)
		return nil
	}
	defer func() { installConnector = old }()
	j := newJob("t", func(*Job) error { return nil })
	if err := ensureProjectTunnel(j, "shop"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(started, ",") != "a:tun-1234567890,b:tun-1234567890" {
		t.Errorf("connectors %v", started)
	}
	typ, content, proxied := m.content("shop.example")
	if typ != "CNAME" || content != "tun-1234567890.cfargotunnel.com" || !proxied {
		t.Errorf("dns %s %s %v", typ, content, proxied)
	}
	if !strings.Contains(string(m.config), `"hostname":"shop.example"`) {
		t.Errorf("config %s", m.config)
	}
	c := snapshotConfig()
	s := findSite(&c, "shop")
	if s.TunnelID != "tun-1234567890" || s.TunnelToken == "" || s.TunnelToken == "secret-token" && protectedIsReal() {
		t.Errorf("tunnel saved: %+v", s)
	}
}

func protectedIsReal() bool { enc, _ := protectString("x"); return enc != "x" }

// ---------- failover ----------

func hs(up bool, oks int) *healthState {
	return &healthState{Known: true, Up: up, OKs: oks}
}

func TestFailoverStateMachine(t *testing.T) {
	order := []string{"a", "b", "c"}
	cfg := FailoverSettings{Auto: true, SwitchBack: true, Active: "a"}
	if d := foStep(cfg, order, map[string]*healthState{"a": hs(true, 5), "b": hs(true, 5)}); d.SwitchTo != "" {
		t.Errorf("healthy: %+v", d)
	}
	if d := foStep(cfg, order, map[string]*healthState{"a": hs(false, 0), "b": hs(false, 0), "c": hs(true, 4)}); d.SwitchTo != "c" || d.Ask {
		t.Errorf("fail to c: %+v", d)
	}
	if d := foStep(cfg, order, map[string]*healthState{"a": hs(false, 0), "b": hs(false, 0), "c": hs(false, 0)}); !d.AllDown {
		t.Errorf("all down: %+v", d)
	}
	cfg.Auto = false
	if d := foStep(cfg, order, map[string]*healthState{"a": hs(false, 0), "b": hs(true, 3)}); d.SwitchTo != "b" || !d.Ask {
		t.Errorf("ask: %+v", d)
	}
	cfg = FailoverSettings{Auto: true, SwitchBack: true, Active: "b"}
	if d := foStep(cfg, order, map[string]*healthState{"a": hs(true, 3), "b": hs(true, 9)}); d.SwitchTo != "a" || d.Reason != "back" {
		t.Errorf("switch back: %+v", d)
	}
	cfg.SwitchBack = false
	if d := foStep(cfg, order, map[string]*healthState{"a": hs(true, 3), "b": hs(true, 9)}); d.SwitchTo != "" {
		t.Errorf("no switch back: %+v", d)
	}
	// Hysteresis: 2 failures keep it up, the third marks it down.
	h := &healthState{}
	h.record(nil, time.Now())
	h.record(errors.New("x"), time.Now())
	h.record(errors.New("x"), time.Now())
	if !h.Up {
		t.Error("down too early")
	}
	h.record(errors.New("x"), time.Now())
	if h.Up {
		t.Error("not down after 3 failures")
	}
	h.record(nil, time.Now())
	h.record(nil, time.Now())
	if h.Up {
		t.Error("up too early")
	}
	h.record(nil, time.Now())
	if !h.Up {
		t.Error("not up after 3 successes")
	}
}

func TestFailoverEndToEndWithMockCloudflare(t *testing.T) {
	isolate(t)
	m := newMockCF(t)
	tok, _ := protectString("cf-token")
	_ = withConfig(func(c *Config) error {
		c.CloudflareToken = tok
		c.Servers = []*ServerProfile{{ID: "a", Label: "Main", ServerIP: "203.0.113.1"}, {ID: "b", Label: "Home", Kind: "home", ServerIP: "192.168.1.9", TunnelID: "hometun"}}
		c.Sites = []*Site{{Slug: "shop", Name: "Shop", Server: "a", Servers: []string{"b"}, HAMode: "backup", Release: "20260101-000000",
			Domain: "shop.example", IncludeWWW: true, CFZoneID: "z1", DNSMode: "cloudflare", Failover: &FailoverSettings{Auto: true, SwitchBack: true}}}
		return nil
	})
	m.records["r1"] = &CFRecord{ID: "r1", Type: "A", Name: "shop.example", Content: "203.0.113.1"}
	m.records["r2"] = &CFRecord{ID: "r2", Type: "A", Name: "www.shop.example", Content: "203.0.113.1"}
	var notes []string
	var nmu sync.Mutex
	oldN := notifyHook
	notifyHook = func(level, site, text string) { nmu.Lock(); notes = append(notes, text); nmu.Unlock() }
	defer func() { notifyHook = oldN }()
	down := map[string]bool{}
	oldP := probeServer
	probeServer = func(c *Config, s *Site, p *ServerProfile) error {
		if down[p.ID] {
			return errors.New("timeout")
		}
		return nil
	}
	defer func() { probeServer = oldP }()
	srvHealthMu.Lock()
	srvHealthMu.m = map[string]map[string]*healthState{}
	srvHealthMu.alerted = map[string]string{}
	srvHealthMu.Unlock()
	round := func() {
		c := snapshotConfig()
		checkProject(&c, findSite(&c, "shop"), time.Now())
	}
	round()
	down["a"] = true
	round()
	round()
	if typ, content, _ := m.content("shop.example"); typ != "A" || content != "203.0.113.1" {
		t.Fatalf("switched too early: %s %s", typ, content)
	}
	round()
	for _, h := range []string{"shop.example", "www.shop.example"} {
		if typ, content, proxied := m.content(h); typ != "CNAME" || content != "hometun.cfargotunnel.com" || !proxied {
			t.Fatalf("%s after failover: %s %s %v", h, typ, content, proxied)
		}
	}
	c := snapshotConfig()
	s := findSite(&c, "shop")
	if s.Failover.Active != "b" || len(s.Failover.Events) != 1 || !strings.Contains(s.Failover.Events[0].Text, "Main to Home") {
		t.Fatalf("failover state %+v", s.Failover)
	}
	// Main recovers: after 3 good checks Rover switches back.
	down["a"] = false
	round()
	round()
	if _, content, _ := m.content("shop.example"); content != "hometun.cfargotunnel.com" {
		t.Fatal("switched back too early")
	}
	round()
	if typ, content, proxied := m.content("shop.example"); typ != "A" || content != "203.0.113.1" || !proxied {
		t.Fatalf("after switch back: %s %s %v", typ, content, proxied)
	}
	nmu.Lock()
	joined := strings.Join(notes, "\n")
	nmu.Unlock()
	if !strings.Contains(joined, "stopped answering") || !strings.Contains(joined, "Switched visitors") || !strings.Contains(joined, "main server is back") {
		t.Errorf("notifications:\n%s", joined)
	}
}

func TestFailoverWithoutCloudflareAsks(t *testing.T) {
	isolate(t)
	_ = withConfig(func(c *Config) error {
		c.Servers = []*ServerProfile{{ID: "a", Label: "Main", ServerIP: "203.0.113.1"}, {ID: "b", Label: "Spare", ServerIP: "203.0.113.2"}}
		c.Sites = []*Site{{Slug: "blog", Name: "Blog", Server: "a", Servers: []string{"b"}, HAMode: "backup", Release: "x", Domain: "blog.example"}}
		return nil
	})
	var notes []string
	oldN := notifyHook
	notifyHook = func(level, site, text string) { notes = append(notes, text) }
	defer func() { notifyHook = oldN }()
	oldP := probeServer
	probeServer = func(c *Config, s *Site, p *ServerProfile) error {
		if p.ID == "a" {
			return errors.New("down")
		}
		return nil
	}
	defer func() { probeServer = oldP }()
	switched := false
	oldS := failoverSwitch
	failoverSwitch = func(*Config, *Site, string) error { switched = true; return nil }
	defer func() { failoverSwitch = oldS }()
	for i := 0; i < 5; i++ {
		c := snapshotConfig()
		checkProject(&c, findSite(&c, "blog"), time.Now())
	}
	if switched {
		t.Error("must not switch without Cloudflare")
	}
	n := 0
	for _, s := range notes {
		if strings.Contains(s, "Switch now") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want exactly one 'Switch now' alert, got %d: %v", n, notes)
	}
}

// ---------- This PC ----------

func TestLocalStaticServerRules(t *testing.T) {
	isolate(t)
	hash := apr1("secret", "abcdefgh")
	enc, _ := protectString(hash)
	_ = withConfig(func(c *Config) error {
		c.ThisPC = &ThisPCConfig{Enabled: true, Port: 18080}
		ensureThisPCProfile(c)
		c.Sites = []*Site{
			{Slug: "shop", Name: "Shop", Server: "a", Servers: []string{thisPCID}, Domain: "shop.example", IncludeWWW: true, Primary: "apex"},
			{Slug: "priv", Name: "Private", Server: "a", Servers: []string{thisPCID}, Access: &SiteAccess{User: "amy", Hash: enc}},
		}
		return nil
	})
	src := t.TempDir()
	_ = os.WriteFile(filepath.Join(src, "_redirects"), []byte("/old /new 301\n/app/* /index.html 200\n"), 0o644)
	_ = os.WriteFile(filepath.Join(src, "_headers"), []byte("/*\n  X-Test: yes\n"), 0o644)
	rememberRules("shop", loadSiteRules("", src))
	tb := tarGz(t, map[string]string{"index.html": "home", "about.html": "about", "404.html": "custom 404", "css/a.css": "body{}"})
	if err := localPublish("shop", tb, "20260101-000000", false); err != nil {
		t.Fatal(err)
	}
	if err := localPublish("priv", tarGz(t, map[string]string{"index.html": "private"}), "20260101-000001", false); err != nil {
		t.Fatal(err)
	}
	h := LocalHost{cfg: snapshotConfig}
	get := func(host, path string, auth string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Host = host
		if auth != "" {
			r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(auth)))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := get("shop.example", "/", ""); w.Code != 200 || w.Body.String() != "home" || w.Header().Get("X-Test") != "yes" {
		t.Errorf("/ -> %d %q %v", w.Code, w.Body.String(), w.Header())
	}
	if w := get("shop.localhost:18080", "/about", ""); w.Code != 200 || w.Body.String() != "about" {
		t.Errorf("/about -> %d %q", w.Code, w.Body.String())
	}
	if w := get("shop.example", "/old", ""); w.Code != 301 || w.Header().Get("Location") != "/new" {
		t.Errorf("/old -> %d %s", w.Code, w.Header().Get("Location"))
	}
	if w := get("shop.example", "/app/deep/link", ""); w.Code != 200 || w.Body.String() != "home" {
		t.Errorf("SPA rewrite -> %d %q", w.Code, w.Body.String())
	}
	if w := get("shop.example", "/nope", ""); w.Code != 404 || w.Body.String() != "custom 404" {
		t.Errorf("404 -> %d %q", w.Code, w.Body.String())
	}
	if w := get("shop.example", "/css/a.css", ""); !strings.HasPrefix(w.Header().Get("Content-Type"), "text/css") {
		t.Errorf("css type %q", w.Header().Get("Content-Type"))
	}
	if w := get("www.shop.example", "/about?x=1", ""); w.Code != 301 || w.Header().Get("Location") != "https://shop.example/about?x=1" {
		t.Errorf("primary redirect -> %d %s", w.Code, w.Header().Get("Location"))
	}
	if w := get("shop.example", "/.dilreet-rules.json", ""); w.Code != 404 || strings.Contains(w.Body.String(), "redirects") {
		t.Errorf("internal file exposed: %d", w.Code)
	}
	if w := get("shop.example", "/../../etc/passwd", ""); w.Code == 200 {
		t.Error("path traversal")
	}
	if w := get("priv.localhost", "/", ""); w.Code != 401 {
		t.Errorf("no auth -> %d", w.Code)
	}
	if w := get("priv.localhost", "/", "amy:wrong"); w.Code != 401 {
		t.Errorf("bad auth -> %d", w.Code)
	}
	if w := get("priv.localhost", "/", "amy:secret"); w.Code != 200 || w.Body.String() != "private" {
		t.Errorf("good auth -> %d %q", w.Code, w.Body.String())
	}
	if w := get("other.example", "/", ""); w.Code != 404 {
		t.Errorf("unknown host -> %d", w.Code)
	}
	// Rollback on This PC is a pointer switch; old releases stay.
	if err := localPublish("shop", tarGz(t, map[string]string{"index.html": "v2"}), "20260102-000000", false); err != nil {
		t.Fatal(err)
	}
	if w := get("shop.example", "/", ""); w.Body.String() != "v2" {
		t.Errorf("after publish %q", w.Body.String())
	}
	if err := localActivate("shop", "20260101-000000"); err != nil {
		t.Fatal(err)
	}
	if w := get("shop.example", "/", ""); w.Body.String() != "home" {
		t.Errorf("after rollback %q", w.Body.String())
	}
	if err := localPublish("shop", tb, "../evil", false); err == nil {
		t.Error("bad release name accepted")
	}
}

func TestExtractRejectsTraversal(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "../escape.txt", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("x"))
	tw.Close()
	gz.Close()
	p := filepath.Join(t.TempDir(), "bad.tgz")
	_ = os.WriteFile(p, buf.Bytes(), 0o644)
	if _, err := extractTarGz(p, t.TempDir()); err == nil {
		t.Error("traversal accepted")
	}
}

func TestNetworkClassification(t *testing.T) {
	list := []netAdapter{
		{Name: "Wi-Fi", Kind: "wifi", Up: true, Gateway: true, Metric: 35, SSID: "Home"},
		{Name: "Ethernet 3", Kind: "usb-tether", Up: true, Gateway: true, Metric: 50},
	}
	if a := activeAdapter(list); a == nil || a.Name != "Wi-Fi" {
		t.Errorf("active %+v", a)
	}
	list[0].Up = false
	if a := activeAdapter(list); a == nil || a.Kind != "usb-tether" || kindLabel(a.Kind) != "phone over USB" {
		t.Errorf("fallback %+v", a)
	}
}

func TestThisPCRoutes(t *testing.T) {
	isolate(t)
	mux := http.NewServeMux()
	registerThisPCRoutes(mux)
	code, out := call(t, mux, "POST", "/api/thispc", map[string]any{"enabled": true, "port": 80})
	if code == 200 {
		t.Errorf("port 80 accepted: %v", out)
	}
	code, out = call(t, mux, "POST", "/api/thispc", map[string]any{"enabled": true, "port": 18234})
	if code != 200 || out["running"] != true {
		t.Fatalf("enable: %d %v", code, out)
	}
	c := snapshotConfig()
	if p := findServer(&c, thisPCID); p == nil || p.Kind != "thispc" {
		t.Error("This PC profile missing")
	}
	if err := activateServer(thisPCID); err == nil {
		t.Error("This PC must not become the active SSH server")
	}
	resp, err := http.Get("http://127.0.0.1:18234/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("no projects yet -> %d", resp.StatusCode)
	}
	code, out = call(t, mux, "POST", "/api/thispc", map[string]any{"enabled": false, "port": 18234})
	if code != 200 || out["running"] != false {
		t.Errorf("disable: %d %v", code, out)
	}
}

// ---------- migration / routes ----------

func TestMigrateV07(t *testing.T) {
	c := &Config{ConfigVersion: 6, Servers: []*ServerProfile{{ID: "x", Provider: "thispc"}}, Sites: []*Site{{Slug: "a", Servers: []string{"x"}}}}
	if !migrateConfig(c) || c.ConfigVersion != currentConfigVersion || c.Servers[0].Kind != "thispc" || c.Sites[0].HAMode != "backup" {
		t.Errorf("migrate: %+v %+v", c, c.Sites[0])
	}
}

func TestReplicaRoutes(t *testing.T) {
	multiSetup(t)
	mux := http.NewServeMux()
	registerV07Routes(mux)
	code, out := call(t, mux, "GET", "/api/sites/shop/replicas", nil)
	if code != 200 || len(out["servers"].([]any)) != 3 || out["mode"] != "backup" || out["can_switch"] != false {
		t.Fatalf("replicas: %d %v", code, out)
	}
	if code, out = call(t, mux, "POST", "/api/sites/shop/servers/a/remove", nil); code == 200 {
		t.Errorf("removing the main server must fail: %v", out)
	}
	if code, _ = call(t, mux, "POST", "/api/sites/shop/servers/c/main", nil); code != 200 {
		t.Error("make main")
	}
	c := snapshotConfig()
	s := findSite(&c, "shop")
	if s.Server != "c" || s.Replicas["a"].AppUUID != "app-a" || strings.Join(s.Servers, ",") != "b,a" {
		t.Errorf("after make main: server=%s servers=%v rep=%+v", s.Server, s.Servers, s.Replicas["a"])
	}
	if code, _ = call(t, mux, "POST", "/api/sites/shop/servers/b/remove", nil); code != 200 {
		t.Error("remove b")
	}
	if code, out = call(t, mux, "POST", "/api/sites/shop/failover/switch", map[string]any{"to": "zzz"}); code == 200 {
		t.Errorf("switch to unknown server: %v", out)
	}
	if code, out = call(t, mux, "POST", "/api/sites/shop/ha", map[string]any{"mode": "weird"}); code == 200 {
		t.Errorf("bad mode accepted: %v", out)
	}
}

func TestSecretsNotInAIContext(t *testing.T) {
	isolate(t)
	tok, _ := protectString("TUNNEL-SECRET-XYZ")
	_ = withConfig(func(c *Config) error {
		c.Servers = []*ServerProfile{{ID: "a", Label: "A", TunnelToken: tok}}
		c.Sites = []*Site{{Slug: "s", Name: "S", Server: "a", TunnelToken: tok}}
		return nil
	})
	ctx := appContext()
	if strings.Contains(ctx, "TUNNEL-SECRET-XYZ") || strings.Contains(ctx, tok) {
		t.Error("tunnel token leaked into the AI/state view")
	}
}
