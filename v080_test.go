package main

// Rover 0.8: guides, plans (do it for me), offline packs, GPU advice and
// Jiv's offline eval set.

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"hostpanel/dlpack"
)

// evalFixture: two projects, three servers, GoDaddy DNS.
func evalFixture(t *testing.T) {
	t.Helper()
	isolate(t)
	withConfig(func(c *Config) error {
		c.ServerIP = "203.0.113.10"
		c.ActiveServer = "s1"
		c.Servers = []*ServerProfile{
			{ID: "s1", Label: "Hetzner main", ServerIP: "203.0.113.10", Kind: "vps"},
			{ID: "s2", Label: "Oracle free", ServerIP: "198.51.100.7", Kind: "vps"},
			{ID: "home1", Label: "Home Pi", ServerIP: "192.168.1.20", Kind: "home", Engine: "lite"},
		}
		c.Sites = []*Site{
			{Slug: "bakery", Name: "Bakery", Server: "s1", Folder: "Bakery"},
			{Slug: "shop", Name: "Shop", Server: "s1", Folder: "Shop", Domain: "shop.com", IncludeWWW: true, HTTPSActive: true, DomainLive: true},
		}
		return nil
	})
	old := planDNSHost
	planDNSHost = func(string) DNSHost { return DNSHost{ID: "godaddy", Name: "GoDaddy"} }
	t.Cleanup(func() { planDNSHost = old })
}

func tools(p *Plan) []string {
	var out []string
	for _, s := range p.Steps {
		out = append(out, s.Tool)
	}
	return out
}

// ---------- eval (no model) ----------

type evalQ struct{ Q, Want string }
type evalT struct {
	Q, Site string
	Want    []string
}

func loadEval(t *testing.T) ([]evalQ, []evalT) {
	var qs []evalQ
	var ts []evalT
	for name, v := range map[string]any{"testdata/jiv-eval-questions.json": &qs, "testdata/jiv-eval-tasks.json": &ts} {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, v); err != nil {
			t.Fatal(name, err)
		}
	}
	return qs, ts
}

// answerIDs is what Jiv would show without a model: the matched
// walkthrough and the top article.
func answerIDs(q string) (guide, art string) {
	if g, _ := matchGuide(q); g != nil {
		guide = g.ID
	}
	if h := jivKB().Search(stripGuideWords(jivNormalize(q)), "", 3); len(h) > 0 {
		art = h[0].ID
	}
	return
}

func TestJivEvalNoModel(t *testing.T) {
	evalFixture(t)
	qs, ts := loadEval(t)
	if len(qs) < 80 || len(ts) < 20 {
		t.Fatalf("eval set too small: %d questions, %d tasks", len(qs), len(ts))
	}
	type row struct {
		Q, Want, Got string
		OK           bool
	}
	var qrows, trows []row
	qok, tok := 0, 0
	for _, c := range qs {
		g, a := answerIDs(c.Q)
		ok := strings.Contains("|"+c.Want+"|", "|"+g+"|") && g != "" || strings.Contains("|"+c.Want+"|", "|"+a+"|")
		if ok {
			qok++
		} else {
			t.Errorf("question %q: guide %q, article %q, want %s", c.Q, g, a, c.Want)
		}
		qrows = append(qrows, row{c.Q, c.Want, g + " / " + a, ok})
	}
	c := snapshotConfig()
	for _, x := range ts {
		p, _, err := makePlan(context.Background(), &c, x.Q, x.Site, false)
		got := ""
		if err != nil {
			got = "error: " + err.Error()
		} else {
			got = strings.Join(tools(p), ",")
		}
		ok := got == strings.Join(x.Want, ",")
		if ok {
			tok++
		} else {
			t.Errorf("task %q: got %s, want %s", x.Q, got, strings.Join(x.Want, ","))
		}
		trows = append(trows, row{x.Q, strings.Join(x.Want, ","), got, ok})
	}
	t.Logf("no-model eval: questions %d/%d, tasks %d/%d", qok, len(qs), tok, len(ts))
	if out := envOr("EVAL_OUT"); out != "" {
		b, _ := json.MarshalIndent(map[string]any{"mode": "no model", "questions_ok": qok, "questions": len(qs), "tasks_ok": tok, "tasks": len(ts), "question_rows": qrows, "task_rows": trows}, "", " ")
		_ = os.WriteFile(out, b, 0o644)
	}
}

func TestPlanArgs(t *testing.T) {
	evalFixture(t)
	c := snapshotConfig()
	p, _, err := makePlan(context.Background(), &c, "put bakery on www.joesbakery.com with https and a backup on my home pc", "", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range p.Steps {
		if s.Tool == "set_domain" && s.Args["domain"] != "joesbakery.com" {
			t.Errorf("domain %v", s.Args)
		}
		if s.Tool == "add_backup_server" && s.Args["server"] != thisPCID {
			t.Errorf("home pc backup should be This PC: %v", s.Args)
		}
		if s.Tool == "dns_records" && !strings.Contains(s.Detail, "walkthrough") {
			t.Errorf("GoDaddy without API key must explain the manual step: %q", s.Detail)
		}
		if sa(s.Args, "site") != "" && sa(s.Args, "site") != "bakery" {
			t.Errorf("site %v", s.Args)
		}
	}
	// home server named explicitly
	p, _, err = makePlan(context.Background(), &c, "add a backup of bakery on my home server", "", false)
	if err != nil || p.Steps[0].Args["server"] != "home1" {
		t.Fatalf("home server backup: %v %v", err, p)
	}
	// validation
	for q, want := range map[string]string{
		"put shop on shop.com with https": "",
		"move bakery to Hetzner main":     "already on",
		"create a project called shop":    "already exists",
	} {
		_, _, err := makePlan(context.Background(), &c, q, "", false)
		if want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
			t.Errorf("%q: err %v, want %q", q, err, want)
		}
	}
	// a domain used by another project is refused
	if _, _, err := makePlan(context.Background(), &c, "put bakery on shop.com with https", "", false); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Errorf("duplicate domain: %v", err)
	}
	// which project?
	if _, d, err := makePlan(context.Background(), &c, "renew the ssl", "", false); !errors.Is(err, errNeedSite) || !d.needSite {
		t.Errorf("need site: %v", err)
	}
}

// ---------- plan runner ----------

type fakeApp struct {
	mu   sync.Mutex
	hits []string
	fail map[string]string // path -> error
	jobs map[string]bool   // path -> answer with a job
}

func (f *fakeApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.hits = append(f.hits, r.Method+" "+r.URL.Path)
	msg, bad := f.fail[r.URL.Path]
	job := f.jobs[r.URL.Path]
	f.mu.Unlock()
	if bad {
		writeJSON(w, 400, map[string]any{"error": msg})
		return
	}
	if job {
		j := newJob("fake", func(j *Job) error { j.Logf("working on %s", r.URL.Path); return nil })
		writeJSON(w, 200, map[string]any{"job": j.ID})
		return
	}
	if r.URL.Path == "/api/sites/bakery/domain" {
		var req struct{ Domain string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		withConfig(func(c *Config) error { findSite(c, "bakery").Domain = req.Domain; return nil })
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (f *fakeApp) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, h := range f.hits {
		if strings.HasPrefix(h, prefix) {
			n++
		}
	}
	return n
}

func planHooks(t *testing.T, f *fakeApp) {
	old := appInner
	appInner = f
	oPoll, oDNS, oHTTPS, oShell, oHealth := planPoll, guideDNSOK, guideHTTPS, planShell, planHealth
	planPoll = 10 * time.Millisecond
	guideDNSOK = func(host, ip string) (bool, string) { return true, "" }
	guideHTTPS = func(slug string) (bool, string) { return true, "ok" }
	planHealth = func(slug string) *HealthResult { d := 80; return &HealthResult{OK: true, CertDays: &d} }
	t.Cleanup(func() {
		appInner = old
		planPoll, guideDNSOK, guideHTTPS, planShell, planHealth = oPoll, oDNS, oHTTPS, oShell, oHealth
	})
}

func waitPlan(t *testing.T, mux http.Handler, id string, until func(p map[string]any) bool) map[string]any {
	t.Helper()
	for i := 0; i < 400; i++ {
		_, out := call(t, mux, "GET", "/api/jiv/plans/"+id, nil)
		p, _ := out["plan"].(map[string]any)
		if p != nil && until(p) {
			return p
		}
		time.Sleep(15 * time.Millisecond)
	}
	_, out := call(t, mux, "GET", "/api/jiv/plans/"+id, nil)
	t.Fatalf("plan did not reach the state: %v", out)
	return nil
}

func stepStates(p map[string]any) string {
	var s []string
	for _, x := range p["steps"].([]any) {
		m := x.(map[string]any)
		s = append(s, fmt.Sprint(m["tool"], "=", m["state"]))
	}
	return strings.Join(s, " ")
}

func planMux() *http.ServeMux {
	mux := http.NewServeMux()
	registerPlanRoutes(mux)
	registerGuideRoutes(mux)
	return mux
}

func TestPlanRunFailureMidwayAndRetry(t *testing.T) {
	evalFixture(t)
	withConfig(func(c *Config) error { c.CloudflareToken = "enc"; return nil })
	planDNSHost = func(string) DNSHost { return DNSHost{ID: "cloudflare", Name: "Cloudflare", Auto: true} }
	f := &fakeApp{fail: map[string]string{"/api/sites/bakery/auto-dns": "Cloudflare: zone not found for joesbakery.com"}, jobs: map[string]bool{}}
	planHooks(t, f)
	mux := planMux()
	code, out := call(t, mux, "POST", "/api/jiv/plan", map[string]any{"q": "put bakery on joesbakery.com with https"})
	if code != 200 {
		t.Fatal(code, out)
	}
	id := out["plan"].(map[string]any)["id"].(string)
	if code, _ := call(t, mux, "POST", "/api/jiv/plans/"+id+"/run", map[string]any{}); code != http.StatusPreconditionRequired {
		t.Fatalf("run without confirm: %d", code)
	}
	if f.count("POST") != 0 {
		t.Fatal("nothing may change before the confirm click")
	}
	call(t, mux, "POST", "/api/jiv/plans/"+id+"/run", map[string]any{"confirmed": true})
	p := waitPlan(t, mux, id, func(p map[string]any) bool { return p["state"] == "failed" })
	if got := stepStates(p); got != "set_domain=done dns_records=failed wait_dns=pending enable_https=pending" {
		t.Fatalf("states: %s", got)
	}
	st := p["steps"].([]any)[1].(map[string]any)
	if !strings.Contains(fmt.Sprint(st["error"]), "zone not found") || st["fix"] == nil {
		t.Fatalf("failed step needs error + fix: %v", st)
	}
	if f.count("POST /api/sites/bakery/domain/") != 0 {
		t.Fatal("later steps must not run after a failure")
	}
	// the user fixes Cloudflare, then Retry
	f.mu.Lock()
	delete(f.fail, "/api/sites/bakery/auto-dns")
	f.mu.Unlock()
	if code, _ := call(t, mux, "POST", "/api/jiv/plans/"+id+"/retry", map[string]any{"confirmed": true}); code != 200 {
		t.Fatal("retry", code)
	}
	p = waitPlan(t, mux, id, func(p map[string]any) bool { return p["state"] == "done" })
	if f.count("POST /api/sites/bakery/domain/apply") != 1 || f.count("POST /api/sites/bakery/auto-dns") != 2 {
		t.Fatalf("calls: %v", f.hits)
	}
	if code, _ := call(t, mux, "POST", "/api/jiv/plans/"+id+"/run", map[string]any{"confirmed": true}); code != 409 {
		t.Fatal("a plan runs once")
	}
}

func TestPlanConfirmGatingRiskySteps(t *testing.T) {
	evalFixture(t)
	f := &fakeApp{fail: map[string]string{}, jobs: map[string]bool{}}
	planHooks(t, f)
	var scripts []string
	var mu sync.Mutex
	planShell = func(ctx context.Context, c *Config, id, script string) (string, error) {
		mu.Lock()
		scripts = append(scripts, script)
		mu.Unlock()
		switch {
		case strings.Contains(script, "HP_DISK"):
			return "HP_DISK 41152736 39000000 2152736\nHP_LOGS 900000\n", nil
		case strings.Contains(script, "HP_BEFORE"):
			return "Removing unused Docker images older than 3 days…\nHP_BEFORE 2152736\nHP_AFTER 9452736 41152736\n", nil
		}
		return "", nil
	}
	mux := planMux()
	_, out := call(t, mux, "POST", "/api/jiv/plan", map[string]any{"q": "free disk space on my server"})
	id := out["plan"].(map[string]any)["id"].(string)
	call(t, mux, "POST", "/api/jiv/plans/"+id+"/run", map[string]any{"confirmed": true})
	p := waitPlan(t, mux, id, func(p map[string]any) bool { return p["state"] == "waiting" })
	if got := stepStates(p); got != "disk_usage=done free_disk=needs_confirm" {
		t.Fatalf("risky step must wait: %s", got)
	}
	if len(scripts) != 1 {
		t.Fatalf("only the read-only check may run: %d scripts", len(scripts))
	}
	st := p["steps"].([]any)[1].(map[string]any)
	tok, _ := st["token"].(string)
	if code, _ := call(t, mux, "POST", "/api/jiv/plans/"+id+"/steps/2/confirm", map[string]any{"confirmed": true, "token": "wrong"}); code != http.StatusPreconditionRequired {
		t.Fatal("wrong token must be refused")
	}
	if code, _ := call(t, mux, "POST", "/api/jiv/plans/"+id+"/steps/2/confirm", map[string]any{"token": tok}); code != http.StatusPreconditionRequired {
		t.Fatal("confirmed=false must be refused")
	}
	call(t, mux, "POST", "/api/jiv/plans/"+id+"/steps/2/confirm", map[string]any{"confirmed": true, "token": tok})
	p = waitPlan(t, mux, id, func(p map[string]any) bool { return p["state"] == "done" })
	res := fmt.Sprint(p["steps"].([]any)[1].(map[string]any)["result"])
	if !strings.Contains(res, "Freed 7.0 GB") {
		t.Fatalf("result %q", res)
	}
	if code, _ := call(t, mux, "POST", "/api/jiv/plans/"+id+"/steps/2/confirm", map[string]any{"confirmed": true, "token": tok}); code == 200 {
		t.Fatal("a token works once")
	}
	if !strings.Contains(scripts[1], "docker image prune -af --filter until=72h") || strings.Contains(scripts[1], "volume") {
		t.Fatal("cleanup must only remove unused images/cache, never volumes")
	}
}

func TestReadOnlyPlanAddsFixNeedingConfirm(t *testing.T) {
	evalFixture(t)
	f := &fakeApp{fail: map[string]string{}, jobs: map[string]bool{"/api/sites/bakery/redeploy-last": true}}
	planHooks(t, f)
	dir, _, _ := projectDir("bakery")
	os.MkdirAll(dir, 0o755)
	writeIndex(dir, &deployIndex{Project: "bakery", Deploys: []*DeployRecord{{ID: "d2", Status: "failed", Error: "i/o timeout while uploading"}, {ID: "d1", Status: "published"}}})
	evs := askStream(t, map[string]any{"q": "why did bakery's deploy fail?"})
	var plan map[string]any
	for _, e := range evs {
		if e["t"] == "plan" {
			plan = e["plan"].(map[string]any)
		}
	}
	if plan == nil || plan["auto"] != true {
		t.Fatalf("read-only plan should start by itself: %v", evs)
	}
	mux := planMux()
	p := waitPlan(t, mux, plan["id"].(string), func(p map[string]any) bool { return p["state"] == "waiting" })
	if got := stepStates(p); got != "why_deploy_failed=done retry_deploy=needs_confirm" {
		t.Fatalf("the added fix must ask first: %s", got)
	}
	if f.count("POST") != 0 {
		t.Fatal("nothing may be published without a click")
	}
}

// ---------- ask pipeline ----------

func TestAskGuideAndPlanEvents(t *testing.T) {
	evalFixture(t)
	f := &fakeApp{fail: map[string]string{}, jobs: map[string]bool{}}
	planHooks(t, f)
	find := func(evs []map[string]any, typ string) map[string]any {
		for _, e := range evs {
			if e["t"] == typ {
				return e
			}
		}
		return nil
	}
	evs := askStream(t, map[string]any{"q": "show me how to connect my godaddy domain", "site": "bakery"})
	g := find(evs, "guide")
	if g == nil || g["guide"].(map[string]any)["id"] != "guide-domain-godaddy" || g["guide"].(map[string]any)["auto"] != true {
		t.Fatalf("explicit show me: %v", evs)
	}
	evs = askStream(t, map[string]any{"q": "how do I add a backup server?"})
	if g := find(evs, "guide"); g == nil || g["guide"].(map[string]any)["auto"] == true {
		t.Fatalf("how-to: guide offered, not started: %v", evs)
	}
	if find(evs, "sources") == nil && find(evs, "text") == nil {
		t.Fatal("the article answer must follow")
	}
	evs = askStream(t, map[string]any{"q": "put bakery on joesbakery.com with https and a backup on this pc"})
	p := find(evs, "plan")
	if p == nil || p["plan"].(map[string]any)["state"] != "ready" {
		t.Fatalf("plan event: %v", evs)
	}
	evs = askStream(t, map[string]any{"q": "renew the ssl"})
	if find(evs, "plansite") == nil {
		t.Fatalf("which project: %v", evs)
	}
	// classic single actions keep their cards
	evs = askStream(t, map[string]any{"q": "roll back shop"})
	if find(evs, "action") == nil || find(evs, "plan") != nil {
		t.Fatalf("roll back shop: %v", evs)
	}
	if f.count("POST") != 0 {
		t.Fatal("asking must not change anything")
	}
}

// ---------- guides ----------

func TestGuidesIntegrity(t *testing.T) {
	evalFixture(t)
	gs := loadGuides()
	if len(gs) < 18 {
		t.Fatalf("only %d guides", len(gs))
	}
	known := regexp.MustCompile(`^(has_server|server_connected|engine_ready|hosting_account|home_server|site_count|deploy_count|release|domain_set|dns_points|domain_live|https_ok|cf_connected|has_replica|thispc_on|brain_ready|ai_ready|update_checked|tcp)$`)
	for _, g := range gs {
		if g.KB != "" && jivKB().Get(g.KB) == nil {
			t.Errorf("%s: kb %q missing", g.ID, g.KB)
		}
		for i, s := range g.Steps {
			switch s.Kind {
			case "ui":
				if s.Sel == "" {
					t.Errorf("%s step %d: ui needs sel", g.ID, i)
				}
			case "external":
				if len(s.Items) == 0 {
					t.Errorf("%s step %d: external needs items", g.ID, i)
				}
				if s.Image != "" {
					if _, err := os.Stat(filepath.Join("web", "guides", s.Image)); err != nil {
						t.Errorf("%s: image %s missing", g.ID, s.Image)
					}
				}
			case "info":
			default:
				t.Errorf("%s step %d: kind %q", g.ID, i, s.Kind)
			}
			for _, spec := range []string{s.SkipIf, s.Verify, func() string {
				if s.Done != nil {
					return s.Done.Check
				}
				return ""
			}()} {
				if spec == "" {
					continue
				}
				name, _, _ := strings.Cut(spec, ":")
				if !known.MatchString(name) {
					t.Errorf("%s step %d: unknown check %q", g.ID, i, spec)
				}
			}
		}
	}
	// the 10 task families of the brief have walkthroughs
	for _, id := range []string{"guide-add-server-oracle", "guide-add-server-hetzner", "guide-add-server-home", "guide-add-project", "guide-deploy-drop", "guide-domain-godaddy", "guide-domain-cloudflare", "guide-https", "guide-backup-server", "guide-this-pc", "guide-restore-deploy", "guide-update-app", "guide-connect-ai"} {
		if guideByID(id) == nil {
			t.Errorf("missing %s", id)
		}
	}
}

func TestGuideResolveAndChecks(t *testing.T) {
	evalFixture(t)
	planHooks(t, &fakeApp{})
	mux := planMux()
	// needs a project
	_, out := call(t, mux, "GET", "/api/jiv/guides/guide-domain-godaddy", nil)
	if out["need_site"] != true {
		t.Fatalf("need site: %v", out)
	}
	_, out = call(t, mux, "GET", "/api/jiv/guides/guide-domain-godaddy?site=bakery", nil)
	g := out["guide"].(map[string]any)
	b, _ := json.Marshal(g)
	if !strings.Contains(string(b), "203.0.113.10") || strings.Contains(string(b), "{ip}") || strings.Contains(string(b), "{site}") {
		t.Fatalf("variables not filled: %s", b)
	}
	n1 := len(g["steps"].([]any))
	// shop already has its domain: the "type your domain" steps are skipped
	_, out = call(t, mux, "GET", "/api/jiv/guides/guide-domain-godaddy?site=shop", nil)
	if n2 := len(out["guide"].(map[string]any)["steps"].([]any)); n2 >= n1 {
		t.Fatalf("skip_if: %d vs %d steps", n2, n1)
	}
	// server checks
	guideDNSOK = func(host, ip string) (bool, string) {
		return host == "shop.com" && ip == "203.0.113.10", "points to 1.2.3.4"
	}
	for spec, want := range map[string]bool{"dns_points:shop": true, "dns_points:bakery": false, "domain_set:shop": true, "site_count": true, "has_server": true, "thispc_on": false, "https_ok:shop": true} {
		_, out := call(t, mux, "POST", "/api/jiv/guides/check", map[string]any{"check": spec})
		if out["ok"] != want {
			t.Errorf("%s: %v", spec, out)
		}
	}
	// no brain: free-form walkthroughs explain how to get one
	code, out := call(t, mux, "POST", "/api/jiv/guides/generate", map[string]any{"q": "how do I make a sandwich"})
	if code != 409 || out["need_brain"] != true {
		t.Fatalf("generate without brain: %d %v", code, out)
	}
	code, out = call(t, mux, "POST", "/api/jiv/guides/generate", map[string]any{"q": "walk me through adding a backup server", "site": "bakery"})
	if code != 200 || out["guide"].(map[string]any)["id"] != "guide-backup-server" {
		t.Fatalf("matched guide first: %d %v", code, out)
	}
}

func TestModelGuideAndPlanWithMockBrain(t *testing.T) {
	evalFixture(t)
	old := brainJSON
	t.Cleanup(func() { brainJSON = old })
	var schemas []map[string]any
	brainJSON = func(ctx context.Context, system, user string, schema map[string]any, maxTokens int) (string, error) {
		schemas = append(schemas, schema)
		if strings.Contains(system, "plan tasks") {
			return `{"steps":[{"tool":"check_services","server":"Oracle free"},{"tool":"restart_service","server":"Oracle free","service":"auto"},{"tool":"rm_rf","server":"x"}]}`, nil
		}
		anchors := guideAnchors()
		return fmt.Sprintf(`{"title":"Open the deploys","steps":[{"anchor":%q,"say":"Click here"},{"anchor":"made.up","say":"x"}]}`, anchors[0].ID), nil
	}
	installMockBrain(t, "ok")
	c := snapshotConfig()
	if _, err := planFromModel(context.Background(), &c, "the oracle box is acting weird, sort it out", ""); err != nil {
		t.Fatal(err)
	}
	d, _ := planFromModel(context.Background(), &c, "the oracle box is acting weird, sort it out", "")
	if _, err := validatePlan(&c, "x", d, "model"); err == nil {
		t.Fatal("an unknown tool from the model must fail validation")
	}
	d.steps = d.steps[:2]
	p, err := validatePlan(&c, "x", d, "model")
	if err != nil || strings.Join(tools(p), ",") != "check_services,restart_service" || p.Steps[1].Args["server"] != "s2" || !p.Steps[1].Risky {
		t.Fatalf("model plan: %v %+v", err, p)
	}
	enum := schemas[0]["properties"].(map[string]any)["steps"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["tool"].(map[string]any)["enum"].([]string)
	if len(enum) != len(planTools) {
		t.Fatal("the grammar must list exactly the plan tools")
	}
	g, err := generateGuide(context.Background(), &c, "where do I see old versions", "bakery")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Steps) != 1 || !g.Generated {
		t.Fatalf("made-up anchors must be dropped: %+v", g)
	}
}

// TestBrainJSONRealEndpoint runs the real brainJSON against the mock sidecar,
// so a wrong URL (it once asked for /v1/v1/chat/completions) fails here.
func TestBrainJSONRealEndpoint(t *testing.T) {
	evalFixture(t)
	installMockBrain(t, "ok")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, err := brainJSON(ctx, "You plan tasks", "check my server", planSchema(), 100)
	if err != nil || !strings.Contains(raw, "check_services") {
		t.Fatalf("brainJSON: %q %v", raw, err)
	}
}

// ---------- offline pack ----------

func fakeCatalog(t *testing.T) (model, cpu, vk string) {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, n int) (string, string, int64) {
		p := filepath.Join(dir, name)
		b := []byte(strings.Repeat(name, n))
		os.WriteFile(p, b, 0o644)
		return p, sha256Hex(b), int64(len(b))
	}
	mp, ms, mz := write("model.gguf", 900)
	cp, cs, cz := zipWith(t, dir, "cpu.zip", map[string]string{serverExeName(): "cpu engine"})
	vp, vs, vz := zipWith(t, dir, "vk.zip", map[string]string{serverExeName(): "vulkan engine", "ggml-vulkan.dll": "x"})
	oldM, oldR := brainModels, brainRuntimes
	brainModels = []BrainModel{{ID: "qwen3-1.7b", Short: "Light", File: "model.gguf", SHA256: ms, Size: mz}}
	brainRuntimes = map[string]brainRuntime{"cpu": {File: "cpu.zip", SHA256: cs, Size: cz}, "vulkan": {File: "vk.zip", SHA256: vs, Size: vz}}
	t.Cleanup(func() { brainModels, brainRuntimes = oldM, oldR })
	return mp, cp, vp
}

func zipWith(t *testing.T, dir, name string, files map[string]string) (string, string, int64) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := writeTestZip(p, files); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	return p, sha256Hex(b), int64(len(b))
}

func runJob(t *testing.T, fn func(j *Job) error) (*Job, error) {
	t.Helper()
	j := newJob("test", fn)
	for i := 0; i < 500; i++ {
		j.mu.Lock()
		done, e := j.done, j.err
		j.mu.Unlock()
		if done {
			if e != "" {
				return j, errors.New(e)
			}
			return j, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job timeout")
	return nil, nil
}

func TestOfflinePackSplitImportExport(t *testing.T) {
	evalFixture(t)
	t.Setenv("ROVER_BRAIN_DIR", t.TempDir())
	t.Setenv("ROVER_BRAIN_UNZIP", "1")
	t.Setenv("ROVER_FAKE_GPU", "NVIDIA GeForce GTX 1650|4096")
	hardware(true)
	t.Cleanup(func() { os.Unsetenv("ROVER_FAKE_GPU"); hardware(true) })
	mp, cp, vp := fakeCatalog(t)
	out := t.TempDir()
	var srcs []dlpack.Source
	srcs = append(srcs, dlpack.Source{Path: mp, File: dlpack.File{Kind: "model", ID: "qwen3-1.7b", SHA256: brainModels[0].SHA256}})
	srcs = append(srcs, dlpack.Source{Path: cp, File: dlpack.File{Kind: "runtime", ID: "cpu", SHA256: brainRuntimes["cpu"].SHA256}})
	srcs = append(srcs, dlpack.Source{Path: vp, File: dlpack.File{Kind: "runtime", ID: "vulkan", SHA256: brainRuntimes["vulkan"].SHA256}})
	files, err := dlpack.Write(filepath.Join(out, "Test.dlpack"), "test", llamaBuild, srcs, 3000, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 3 || !strings.HasSuffix(files[0], ".dlpack.001") {
		t.Fatalf("expected a split pack: %v", files)
	}
	// any part (or the folder) works
	if _, err := runJob(t, func(j *Job) error { return importPackJob(j, files[1]) }); err != nil {
		t.Fatal(err)
	}
	c := snapshotConfig()
	js := jivSettings(&c)
	if js.Model != "qwen3-1.7b" || !js.GPU {
		t.Fatalf("settings after import: %+v", js)
	}
	exe, _, _, ok := brainPaths(&c)
	if !ok || !strings.HasSuffix(filepath.Dir(exe), "-vulkan") {
		t.Fatalf("GPU uses the Vulkan engine: %s %v", exe, ok)
	}
	b, _ := os.ReadFile(exe)
	if string(b) != "vulkan engine" {
		t.Fatalf("engine content %q", b)
	}
	// export, then import that on a "new PC"
	exp := t.TempDir()
	if _, err := runJob(t, func(j *Job) error { return exportPackJob(j, exp) }); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ROVER_BRAIN_DIR", t.TempDir())
	withConfig(func(c *Config) error { c.Jiv = nil; return nil })
	if _, err := runJob(t, func(j *Job) error { return importPackJob(j, exp) }); err != nil {
		t.Fatal("import from folder:", err)
	}
	// a missing part
	os.Remove(files[len(files)-1])
	if _, err := runJob(t, func(j *Job) error { return importPackJob(j, files[0]) }); err == nil {
		t.Fatal("missing part must fail")
	}
}

func TestOfflinePackRejectsUnknownAndDamaged(t *testing.T) {
	evalFixture(t)
	t.Setenv("ROVER_BRAIN_DIR", t.TempDir())
	mp, _, _ := fakeCatalog(t)
	dir := t.TempDir()
	evil := filepath.Join(dir, "llama-server.exe")
	os.WriteFile(evil, []byte("not pinned"), 0o755)
	files, _ := dlpack.Write(filepath.Join(dir, "evil.dlpack"), "evil", llamaBuild, []dlpack.Source{{Path: evil, File: dlpack.File{Kind: "runtime", ID: "cpu", SHA256: sha256Hex([]byte("not pinned"))}}}, 0, nil)
	if _, err := runJob(t, func(j *Job) error { return importPackJob(j, files[0]) }); err == nil || !strings.Contains(err.Error(), "nothing Rover can use") {
		t.Fatalf("unknown files must be refused: %v", err)
	}
	// manifest claims the pinned checksum, content differs
	fake := filepath.Join(dir, "model.gguf")
	b, _ := os.ReadFile(mp)
	b[10] ^= 1
	os.WriteFile(fake, b, 0o644)
	files, err := dlpack.Write(filepath.Join(dir, "damaged.dlpack"), "x", llamaBuild, []dlpack.Source{{Path: fake, File: dlpack.File{Kind: "model", ID: "qwen3-1.7b"}}}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	pk, _ := dlpack.Open(files[0])
	pk.Close()
	// rewrite the manifest checksum to the pinned one
	tampered := filepath.Join(dir, "tampered.dlpack")
	raw, _ := os.ReadFile(files[0])
	got := sha256Hex(b)
	raw = []byte(strings.Replace(string(raw), `"sha256": ""`, `"sha256": "`+brainModels[0].SHA256+`"`, 1))
	if !strings.Contains(string(raw), brainModels[0].SHA256) {
		_ = got
		t.Skip("manifest layout changed")
	}
	os.WriteFile(tampered, raw, 0o644)
	if _, err := runJob(t, func(j *Job) error { return importPackJob(j, tampered) }); err == nil {
		t.Fatal("damaged content must be refused")
	}
	bd, _ := brainDir()
	if fileExists(filepath.Join(bd, "model.gguf")) {
		t.Fatal("a damaged file must not stay installed")
	}
}

// ---------- GPU ----------

func TestGPURecommendation(t *testing.T) {
	g := func(name string, mib uint64) GPUInfo {
		x := GPUInfo{Name: name, Vendor: vendorOf("", name), VRAM: mib << 20}
		x.Dedicated = isDedicated(x)
		return x
	}
	cases := []struct {
		hw    HWInfo
		model string
		gpu   bool
	}{
		{HWInfo{GPUs: []GPUInfo{g("NVIDIA GeForce GTX 1650", 4096), g("AMD Radeon(TM) Graphics", 512)}, Vulkan: true, RAM: 8 << 30, Cores: 12}, "qwen3-4b", true}, // the user's laptop
		{HWInfo{GPUs: []GPUInfo{g("NVIDIA GeForce MX450", 2048)}, Vulkan: true, RAM: 8 << 30, Cores: 8}, "qwen3-1.7b", true},
		{HWInfo{GPUs: []GPUInfo{g("NVIDIA GeForce GTX 1650", 4096)}, Vulkan: false, RAM: 8 << 30, Cores: 12}, "qwen3-1.7b", false},
		{HWInfo{GPUs: []GPUInfo{g("Intel(R) UHD Graphics", 128)}, Vulkan: true, RAM: 16 << 30, Cores: 8}, "qwen3-4b", false},
		{HWInfo{GPUs: []GPUInfo{g("Intel(R) UHD Graphics", 128)}, Vulkan: true, RAM: 8 << 30, Cores: 8}, "qwen3-1.7b", false},
		{HWInfo{GPUs: []GPUInfo{g("AMD Radeon RX 6600", 8192)}, Vulkan: true, RAM: 16 << 30, Cores: 12}, "qwen3-4b", true},
		{HWInfo{GPUs: []GPUInfo{g("Intel(R) Arc(TM) A370M Graphics", 4096)}, Vulkan: true, RAM: 16 << 30, Cores: 12}, "qwen3-4b", true},
		{HWInfo{GPUs: []GPUInfo{g("Microsoft Basic Display Adapter", 0)}, Vulkan: false, RAM: 4 << 30, Cores: 2}, "qwen3-1.7b", false},
	}
	for i, c := range cases {
		r := recommendFor(c.hw)
		if r.Model != c.model || r.GPU != c.gpu {
			t.Errorf("case %d (%s): got %s gpu=%v (%s)", i, c.hw.GPUs[0].Name, r.Model, r.GPU, r.Why)
		}
	}
	if a := strings.Join(brainGPUArgs(true), " "); !strings.Contains(a, "--fit on") || strings.Contains(a, "-ngl") || !strings.Contains(a, "-ctk q8_0") {
		t.Fatalf("GPU args must let --fit work (no -ngl): %s", a)
	}
}

func TestBrainGPUFallbackToCPU(t *testing.T) {
	isolate(t)
	argFile := installMockBrain(t, "gpucrash")
	withConfig(func(c *Config) error { c.Jiv.GPU = true; return nil })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, _, err := brainSidecar.Ensure(ctx); err != nil {
		t.Fatal("should fall back to the CPU:", err)
	}
	st := brainSidecar.Status()
	if st["gpu"] != false || !strings.Contains(fmt.Sprint(st["note"]), "processor") {
		t.Fatalf("status %v", st)
	}
	a, _ := os.ReadFile(argFile)
	if !strings.Contains(string(a), "-dev none") {
		t.Fatalf("CPU run args: %s", a)
	}
}

func sha256Hex(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func writeTestZip(p string, files map[string]string) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	for n, c := range files {
		w, err := zw.Create(n)
		if err != nil {
			return err
		}
		io.WriteString(w, c)
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}
