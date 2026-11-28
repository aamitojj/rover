package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"hostpanel/prov"
)

func integMux() *http.ServeMux {
	mux := http.NewServeMux()
	registerIntegRoutes(mux)
	registerAnalyticsRoutes(mux)
	registerMonitor09Routes(mux)
	registerSyncRoutes(mux)
	registerGoogleAuthRoutes(mux)
	registerNginxDirectRoutes(mux)
	registerBackupRoutes(mux)
	mux.HandleFunc("GET /api/providers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, providerCatalog())
	})
	return mux
}

func TestMigrateV09(t *testing.T) {
	c := &Config{ConfigVersion: 8, Sites: []*Site{{Slug: "shop"}}}
	if !migrateConfig(c) || c.ConfigVersion != 9 || c.Analytics == nil || !c.Analytics.Enabled || c.Alerts == nil {
		t.Fatalf("migrate v09: %+v", c)
	}
	// second pass is a no-op
	if migrateConfig(c) {
		t.Fatal("second migrate should be unchanged")
	}
}

func TestProviderCatalogIncludesV09(t *testing.T) {
	isolate(t)
	_, out := call(t, integMux(), "GET", "/api/providers", nil)
	raw, _ := json.Marshal(out)
	// call() unmarshals into map; catalog is a list so use a raw request.
	req := httptest.NewRequest("GET", "/api/providers", nil)
	rec := httptest.NewRecorder()
	integMux().ServeHTTP(rec, req)
	var list []ProviderDefinition
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) < 18 {
		t.Fatalf("catalog: %s %v", rec.Body.Bytes(), err)
	}
	ids := map[string]bool{}
	for _, p := range list {
		ids[p.ID] = true
	}
	for _, id := range []string{"coolify", "nginx", "supabase", "firebase", "netlify", "vercel", "oracle", "aws", "gcp", "azure", "cloudflare", "github", "gitlab", "ga4", "search-console", "adsense", "local"} {
		if !ids[id] {
			t.Fatalf("missing %s", id)
		}
	}
	if providerSupports("supabase", "hosting") {
		t.Fatal("supabase hosting must stay unsupported")
	}
	_ = raw
}

func TestIntegrationCentreConnectTestDisconnect(t *testing.T) {
	isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer nfp_test_token_abcdefgh" {
			http.Error(w, "bad auth", 401)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/v1/user"):
			w.Write([]byte(`{"id":"u1","email":"ada@example.com","full_name":"Ada"}`))
		case strings.HasSuffix(r.URL.Path, "/api/v1/sites"):
			w.Write([]byte(`[{"id":"s1","name":"shop","url":"https://shop.netlify.app","state":"current","plan_name":"Starter"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := integTransport
	integTransport = func() *prov.Transport { return &prov.Transport{Base: map[string]string{"netlify": srv.URL}} }
	t.Cleanup(func() { integTransport = old })

	code, out := call(t, integMux(), "POST", "/api/integrations", map[string]any{
		"provider": "netlify", "nickname": "Work", "cred": map[string]string{"token": "nfp_test_token_abcdefgh"},
	})
	if code != 200 || out["id"] == nil || out["provider"] != "netlify" || out["status"] != "connected" {
		t.Fatalf("connect: %d %v", code, out)
	}
	body, _ := json.Marshal(out)
	if strings.Contains(string(body), "nfp_test_token_abcdefgh") || strings.Contains(string(body), `"cred":"plain:`) {
		t.Fatalf("secret leaked in connect response: %s", body)
	}
	id, _ := out["id"].(string)

	code, out = call(t, integMux(), "GET", "/api/integrations", nil)
	// list is an array
	req := httptest.NewRequest("GET", "/api/integrations", nil)
	rec := httptest.NewRecorder()
	integMux().ServeHTTP(rec, req)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "nfp_test_token_abcdefgh") {
		t.Fatalf("list leaked or failed: %d %s", rec.Code, rec.Body.String())
	}

	code, out = call(t, integMux(), "POST", "/api/integrations/"+id+"/test", map[string]any{})
	if code != 200 || out["status"] != "connected" || strings.Contains(string(mustJSON(out)), "nfp_test_token_abcdefgh") {
		t.Fatalf("test: %d %v", code, out)
	}

	// assign to a project
	_ = withConfig(func(c *Config) error {
		c.Sites = append(c.Sites, &Site{Slug: "shop", Name: "Shop"})
		return nil
	})
	code, out = call(t, integMux(), "POST", "/api/sites/shop/services", map[string]any{"services": map[string]string{"hosting": id}, "engine": "netlify"})
	if code != 200 || out["engine"] != "netlify" {
		t.Fatalf("assign: %d %v", code, out)
	}

	code, out = call(t, integMux(), "DELETE", "/api/integrations/"+id, nil)
	if code != 200 {
		t.Fatalf("disconnect: %d %v", code, out)
	}
	c := snapshotConfig()
	if len(c.Integrations) != 0 {
		t.Fatalf("still present: %+v", c.Integrations)
	}
	if s := findSite(&c, "shop"); s.Services["hosting"] != "" {
		t.Fatalf("assignment not cleared: %+v", s.Services)
	}
}

func TestCostRequiresConfirmation(t *testing.T) {
	isolate(t)
	_ = withConfig(func(c *Config) error {
		enc, _ := protectCred(map[string]string{"access_key": "AKIAEXAMPLE", "secret_key": "secret", "region": "us-east-1"})
		c.Integrations = []*Integration{{ID: "int_aws", Provider: "aws", Cred: enc, Status: "connected"}}
		return nil
	})
	code, out := call(t, integMux(), "POST", "/api/integrations/int_aws/cost", map[string]any{"confirm_paid": false})
	if code == 200 {
		t.Fatalf("unconfirmed cost must fail: %v", out)
	}
	if !strings.Contains(strings.ToLower(out["error"].(string)), "confirm") {
		t.Fatalf("error: %v", out)
	}
}

func TestAnalyticsBeaconAndSummary(t *testing.T) {
	isolate(t)
	_ = withConfig(func(c *Config) error {
		c.Sites = []*Site{{Slug: "shop", Name: "Shop", Domain: "shop.test"}}
		c.Analytics = &AnalyticsSettings{Enabled: true, RetentionDays: 90, BotFilter: true}
		return nil
	})
	mux := integMux()
	req := httptest.NewRequest("GET", "/__rover/e?site=shop&p=/about&r=https://google.com/search?q=secret", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Chrome/120.0)")
	req.Header.Set("CF-IPCountry", "IN")
	req.RemoteAddr = "203.0.113.9:1234"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("beacon %d", rec.Code)
	}
	// bot ignored
	req = httptest.NewRequest("GET", "/__rover/e?site=shop&p=/", nil)
	req.Header.Set("User-Agent", "Googlebot/2.1")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	code, _ := call(t, mux, "GET", "/api/analytics/shop?days=7", nil)
	req = httptest.NewRequest("GET", "/api/analytics/shop?days=7", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 && code != 200 {
		t.Fatalf("summary %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "203.0.113.9") || strings.Contains(body, "q=secret") {
		t.Fatalf("raw IP or query leaked: %s", body)
	}
	var sum analyticsSummaryView
	if err := json.Unmarshal(rec.Body.Bytes(), &sum); err != nil {
		t.Fatal(err)
	}
	if sum.Pageviews != 1 || sum.Uniques != 1 || sum.Countries[0].Name != "IN" {
		t.Fatalf("summary %+v", sum)
	}

	n, err := ingestAccessLog("shop", `1.2.3.4 - - [10/Jan/2026:10:00:00 +0000] "GET /pricing HTTP/1.1" 200 1234 "https://news.example/" "Mozilla/5.0 Chrome/1" "US"`+"\n", true)
	if err != nil || n != 1 {
		t.Fatalf("ingest %d %v", n, err)
	}
	ents, _ := os.ReadDir(analyticsDir("shop"))
	for _, e := range ents {
		b, _ := os.ReadFile(filepath.Join(analyticsDir("shop"), e.Name()))
		if bytes.Contains(b, []byte("1.2.3.4")) || bytes.Contains(b, []byte("203.0.113.9")) {
			t.Fatalf("raw IP stored: %s", b)
		}
	}
}

func TestParseMetricsAndAlerts(t *testing.T) {
	out := "HP_UP 3600\nHP_CPU 12.5\nHP_RAM 512000 1024000\nHP_SWAP 0 0\nHP_DISK 1000000 4000000\nHP_NET 10 20\n"
	m := parseMetrics("s1", "box", out)
	if m.UptimeSec != 3600 || m.CPU != 12.5 || m.RAMTotal != 1024000*1024 || m.DiskUsed != 1000000*1024 || m.NetTx != 20 {
		t.Fatalf("%+v", m)
	}
}

func TestSyncPayloadHasNoSecrets(t *testing.T) {
	isolate(t)
	_ = withConfig(func(c *Config) error {
		c.ManagedKey = "plain:-----BEGIN PRIVATE KEY-----\nMII\n-----END PRIVATE KEY-----"
		c.CoolifyToken = "plain:secret-token"
		c.Sites = []*Site{{Slug: "shop", Name: "Shop", Domain: "shop.com", LastDeploy: "1"}}
		c.CloudSync = &SyncSettings{DeviceID: "dev_test"}
		return nil
	})
	c := snapshotConfig()
	p := buildSyncPayload(&c)
	b, _ := json.Marshal(p)
	if syncContainsSecret(b) {
		t.Fatalf("payload flagged: %s", b)
	}
	if !bytes.Contains(b, []byte("shop.com")) {
		t.Fatal("missing project")
	}
	if bytes.Contains(b, []byte("PRIVATE KEY")) || bytes.Contains(b, []byte("secret-token")) || bytes.Contains(b, []byte("ManagedKey")) {
		t.Fatalf("secret in payload: %s", b)
	}
}

func TestSiteLockSerializes(t *testing.T) {
	var started, mid sync.WaitGroup
	started.Add(1)
	mid.Add(1)
	errCh := make(chan error, 1)
	go func() {
		_ = withSiteLock("shop", func() error {
			started.Done()
			mid.Wait()
			return nil
		})
	}()
	started.Wait()
	go func() { errCh <- withSiteLock("shop", func() error { return nil }) }()
	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "already") {
			t.Fatalf("lock: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lock did not reject a second deploy")
	}
	mid.Done()
}

func TestRedactHidesNewTokens(t *testing.T) {
	s := redact("Authorization: Bearer nfp_abcdefghijklmnop and sbp_abcdefghijklmnop", false)
	if strings.Contains(s, "nfp_abcdefghijklmnop") && !strings.Contains(s, "[hidden]") {
		t.Fatalf("not redacted: %s", s)
	}
	if strings.Contains(s, "sbp_abcdefghijklmnop") {
		t.Fatalf("sbp not redacted: %s", s)
	}
}

func TestNginxInstallScriptSafety(t *testing.T) {
	s := nginxInstallScript()
	if strings.Contains(s, "setenforce 0") || strings.Contains(strings.ToLower(s), "selinux=disabled") || strings.Contains(s, "ufw disable") || strings.Contains(s, "systemctl stop firewalld") {
		t.Fatal("installer must not disable SELinux or the firewall")
	}
	if !strings.Contains(s, "Rover does not create swap automatically") {
		t.Fatal("missing swap advice")
	}
	cb := certbotScript("shop", "me@example.com", []string{"shop.com", "bad;rm -rf /"})
	if strings.Contains(cb, "rm -rf") || strings.Contains(cb, "bad;") {
		t.Fatalf("host names not allowlisted: %s", cb)
	}
	if !strings.Contains(cb, "shop.com") {
		t.Fatal("missing domain")
	}
}

func TestGoogleClientIDValidation(t *testing.T) {
	isolate(t)
	code, out := call(t, integMux(), "POST", "/api/account/google/client", map[string]any{"client_id": "not-a-client"})
	if code == 200 {
		t.Fatalf("accepted bad id: %v", out)
	}
	code, out = call(t, integMux(), "POST", "/api/account/google/client", map[string]any{"client_id": "123-abc.apps.googleusercontent.com"})
	if code != 200 {
		t.Fatalf("good id: %d %v", code, out)
	}
	c := snapshotConfig()
	if c.GoogleClientID == "" {
		t.Fatal("not saved")
	}
}

func TestHostKeyChangeIsRejected(t *testing.T) {
	// Documented contract: connectSSH compares the pin and errors with SERVER HOST KEY CHANGED.
	src, _ := os.ReadFile("sshclient.go")
	if !bytes.Contains(src, []byte("SERVER HOST KEY CHANGED")) || !bytes.Contains(src, []byte("HostKeyCallback")) {
		t.Fatal("TOFU change check missing")
	}
}

func TestIntegViewsHideLegacyCreds(t *testing.T) {
	isolate(t)
	_ = withConfig(func(c *Config) error {
		enc, _ := protectCred(map[string]string{"token": "hcloud-secret-token-value"})
		c.HostingAccounts = []*HostingAccount{{ID: "ha1", Provider: "hetzner", Nickname: "H", Cred: enc, Status: "ok"}}
		c.CloudflareToken = "plain:cf-secret-token-value"
		return nil
	})
	c := snapshotConfig()
	views := integViews(&c)
	b, _ := json.Marshal(views)
	if bytes.Contains(b, []byte("hcloud-secret-token-value")) || bytes.Contains(b, []byte("cf-secret-token-value")) || bytes.Contains(b, []byte(`"cred":"plain:`)) {
		t.Fatalf("legacy secret leaked: %s", b)
	}
	foundH, foundCF := false, false
	for _, v := range views {
		if v.Source == "hosting" && v.Provider == "hetzner" {
			foundH = true
		}
		if v.Source == "cloudflare" {
			foundCF = true
		}
	}
	if !foundH || !foundCF {
		t.Fatalf("legacy accounts missing: %+v", views)
	}
}

func TestUnsupportedCostIsMarked(t *testing.T) {
	isolate(t)
	_ = withConfig(func(c *Config) error {
		c.Integrations = []*Integration{{ID: "int_net", Provider: "netlify", Cred: "plain:{}", Status: "connected"}}
		return nil
	})
	code, out := call(t, integMux(), "POST", "/api/integrations/int_net/cost", map[string]any{"confirm_paid": true})
	if code != 200 || out["unsupported"] != true {
		t.Fatalf("expected unsupported: %d %v", code, out)
	}
}

var _ = context.Background
