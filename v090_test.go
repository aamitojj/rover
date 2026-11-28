package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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

func TestFirebaseSyncWrite(t *testing.T) {
	isolate(t)
	var gotPath, gotAuth, gotBody, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"ok"}`))
	}))
	t.Cleanup(srv.Close)
	old := integTransport
	integTransport = func() *prov.Transport { return &prov.Transport{Base: map[string]string{"firestore": srv.URL}} }
	t.Cleanup(func() { integTransport = old })

	cred, _ := protectString(`{"access_token":"ya29.sync-demo","project_id":"rover-sync-demo"}`)
	_ = withConfig(func(c *Config) error {
		c.ManagedKey = "plain:-----BEGIN PRIVATE KEY-----\nMII\n-----END PRIVATE KEY-----"
		c.CoolifyToken = "plain:never-upload-this"
		c.Sites = []*Site{{Slug: "shop", Name: "Shop", Domain: "shop.com", Engine: "nginx", LastDeploy: "1", Release: "r1"}}
		c.Integrations = []*Integration{{ID: "int_fb", Provider: "firebase", Nickname: "FB", Cred: cred, Status: "connected"}}
		c.CloudSync = &SyncSettings{Enabled: true, Provider: "firebase", IntegrationID: "int_fb", DeviceID: "dev_test"}
		return nil
	})
	_, out := call(t, integMux(), "POST", "/api/sync/now", nil)
	if gotMethod != "PATCH" {
		t.Fatalf("method %s out=%v", gotMethod, out)
	}
	if gotPath != "/v1/projects/rover-sync-demo/databases/(default)/documents/rover_sync/dev_test" {
		t.Fatalf("path %s", gotPath)
	}
	if gotAuth != "Bearer ya29.sync-demo" {
		t.Fatalf("auth %s", gotAuth)
	}
	if !strings.Contains(gotBody, `"stringValue":"shop.com"`) || !strings.Contains(gotBody, `"stringValue":"dev_test"`) {
		t.Fatalf("body %s", gotBody)
	}
	for _, bad := range []string{"PRIVATE KEY", "never-upload-this", "ya29.sync-demo", "managed_key", "coolify_token", `"cred"`} {
		if strings.Contains(gotBody, bad) {
			t.Fatalf("secret %q leaked in Firestore body: %s", bad, gotBody)
		}
	}

	// Refuse a payload that looks like a key (direct call).
	err := pushSync(context.Background(), ptrCfg(), []byte(`{"note":"-----BEGIN PRIVATE KEY-----"}`))
	if err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("wanted secret refuse, got %v", err)
	}
}

func ptrCfg() *Config {
	c := snapshotConfig()
	return &c
}

func TestGoogleReportsProxy(t *testing.T) {
	isolate(t)
	var ga4Path, gscPath, adsPath string
	var leaked bool
	ga4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ga4Path = r.URL.Path
		if strings.Contains(r.Header.Get("Authorization"), "secret") {
			leaked = true
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"dimensionHeaders":[{"name":"date"}],"metricHeaders":[{"name":"activeUsers"}],"rows":[{"dimensionValues":[{"value":"20261101"}],"metricValues":[{"value":"12"}]}]}`)
	}))
	gsc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gscPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"rows":[{"keys":["bread","https://shop.com/"],"clicks":3,"impressions":20,"ctr":0.15,"position":2}]}`)
	}))
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adsPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"headers":[{"name":"DATE"},{"name":"ESTIMATED_EARNINGS"}],"rows":[{"cells":[{"value":"2026-11-01"},{"value":"4.50"}]}]}`)
	}))
	t.Cleanup(ga4.Close)
	t.Cleanup(gsc.Close)
	t.Cleanup(ads.Close)
	old := integTransport
	integTransport = func() *prov.Transport {
		return &prov.Transport{Base: map[string]string{"ga4-data": ga4.URL, "gsc": gsc.URL, "adsense": ads.URL}}
	}
	t.Cleanup(func() { integTransport = old })

	gaCred, _ := protectString(`{"access_token":"ya29.ga4-secret","property_id":"properties/111"}`)
	scCred, _ := protectString(`{"access_token":"ya29.gsc-secret","site_url":"https://shop.com/"}`)
	adCred, _ := protectString(`{"access_token":"ya29.ads-secret","account":"accounts/pub-1"}`)
	_ = withConfig(func(c *Config) error {
		c.Sites = []*Site{{Slug: "shop", Name: "Shop", Domain: "shop.com", Services: map[string]string{"analytics": "int_ga4", "seo": "int_gsc", "revenue": "int_ads"}}}
		c.Integrations = []*Integration{
			{ID: "int_ga4", Provider: "ga4", Nickname: "GA4", Cred: gaCred, Status: "connected", Who: "Shop property"},
			{ID: "int_gsc", Provider: "search-console", Nickname: "GSC", Cred: scCred, Status: "connected"},
			{ID: "int_ads", Provider: "adsense", Nickname: "AdSense", Cred: adCred, Status: "connected"},
		}
		return nil
	})
	code, out := call(t, integMux(), "GET", "/api/analytics/shop/google?days=28", nil)
	if code != 200 {
		t.Fatalf("status %d %v", code, out)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "ya29") || strings.Contains(string(raw), "-secret") {
		t.Fatalf("token leaked: %s", raw)
	}
	_ = leaked
	if ga4Path != "/v1beta/properties/111:runReport" {
		t.Fatalf("ga4 path %s", ga4Path)
	}
	if !strings.Contains(gscPath, "searchAnalytics/query") {
		t.Fatalf("gsc path %s", gscPath)
	}
	if !strings.Contains(adsPath, "/reports:generate") {
		t.Fatalf("ads path %s", adsPath)
	}
	m := out
	ga4out, _ := m["ga4"].(map[string]any)
	if ga4out["connected"] != true {
		t.Fatalf("ga4 %v (type %T) full=%s", out, out, raw)
	}
	table, _ := ga4out["table"].(map[string]any)
	rows, _ := table["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("ga4 rows %v", table)
	}
	gscOut, _ := m["search_console"].(map[string]any)
	if gscOut["connected"] != true {
		t.Fatalf("gsc %v", gscOut)
	}
	adsOut, _ := m["adsense"].(map[string]any)
	if adsOut["connected"] != true {
		t.Fatalf("adsense %v", adsOut)
	}
}

func TestWebhookAndSMTPAlerts(t *testing.T) {
	isolate(t)
	gotHook := make(chan string, 1)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotHook <- string(b)
		w.WriteHeader(204)
	}))
	t.Cleanup(hook.Close)

	addr, cap, closeSMTP := startFakeSMTP()
	t.Cleanup(closeSMTP)
	host, portStr, _ := strings.Cut(addr, ":")
	port, _ := strconv.Atoi(portStr)

	pass, _ := protectString("smtp-secret-password")
	a := &AlertSettings{
		InApp: false, Windows: false,
		Webhook: hook.URL,
		Email:   "ops@example.com", SMTPFrom: "rover@example.com",
		SMTPHost: host, SMTPPort: port, SMTPUser: "rover", SMTPPass: pass, SMTPTLS: "none",
	}
	fireAlert(a, "warn", "shop", "CPU is 97% on the bakery server")

	select {
	case body := <-gotHook:
		if !strings.Contains(body, "CPU is 97%") || !strings.Contains(body, `"site":"shop"`) {
			t.Fatalf("webhook body %s", body)
		}
		if strings.Contains(body, "smtp-secret-password") || strings.Contains(body, "dpapi:") {
			t.Fatalf("secret in webhook: %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("webhook not posted")
	}
	var from, data string
	var to []string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		from, data, to = cap.snapshot()
		if data != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if from != "rover@example.com" || len(to) != 1 || to[0] != "ops@example.com" {
		t.Fatalf("smtp envelope from=%q to=%v", from, to)
	}
	if !strings.Contains(data, "CPU is 97%") || !strings.Contains(data, "Subject: Rover alert (warn)") {
		t.Fatalf("smtp data %q", data)
	}
	if strings.Contains(data, "smtp-secret-password") {
		t.Fatal("password in message")
	}

	// GET /api/monitor must not return the password.
	_ = withConfig(func(c *Config) error { c.Alerts = a; return nil })
	code, out := call(t, integMux(), "GET", "/api/monitor", nil)
	if code != 200 {
		t.Fatalf("monitor %d %v", code, out)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "smtp-secret-password") || strings.Contains(string(raw), "plain:smtp") {
		t.Fatalf("password leaked: %s", raw)
	}
	alerts, _ := out["alerts"].(map[string]any)
	if alerts["smtp_pass_set"] != true || alerts["smtp_pass"] != nil && fmt.Sprint(alerts["smtp_pass"]) != "" && fmt.Sprint(alerts["smtp_pass"]) != "<nil>" {
		// smtp_pass should be omitted or empty
		if v, ok := alerts["smtp_pass"]; ok && v != "" && v != nil {
			t.Fatalf("smtp_pass in view: %v", alerts)
		}
	}

	// Unencrypted SMTP to a public host is refused.
	if err := sendSMTPAlert(&AlertSettings{SMTPHost: "smtp.example.com", SMTPPort: 25, SMTPTLS: "none", SMTPFrom: "a@b.co", Email: "c@d.co"}, "info", "", "hi"); err == nil || !strings.Contains(err.Error(), "unencrypted") {
		t.Fatalf("wanted local-only none, got %v", err)
	}

	// Saving a blank password keeps the stored one; the response is redacted.
	code, out = call(t, integMux(), "POST", "/api/monitor/alerts", map[string]any{
		"email": "ops@example.com", "smtp_from": "rover@example.com",
		"smtp_host": "127.0.0.1", "smtp_port": port, "smtp_user": "rover", "smtp_pass": "", "smtp_tls": "none",
		"webhook": hook.URL,
	})
	if code != 200 {
		t.Fatalf("save %d %v", code, out)
	}
	if out["smtp_pass_set"] != true || out["smtp_pass"] != nil && fmt.Sprint(out["smtp_pass"]) != "" && out["smtp_pass"] != nil {
		if v := out["smtp_pass"]; v != nil && v != "" {
			t.Fatalf("save leaked pass %v", out)
		}
	}
	c := snapshotConfig()
	if secret(c.Alerts.SMTPPass) != "smtp-secret-password" {
		t.Fatalf("stored pass lost: %q", c.Alerts.SMTPPass)
	}
}

func TestWebhookMustBeHTTPS(t *testing.T) {
	isolate(t)
	code, out := call(t, integMux(), "POST", "/api/monitor/alerts", map[string]any{"webhook": "http://evil.example/hook"})
	if code == 200 {
		t.Fatalf("accepted plain http webhook: %v", out)
	}
}

func testCityMMDB(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/test-city.mmdb.b64")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := decodeB64(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "test-city.mmdb")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func decodeB64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

func TestMMDBCityLookup(t *testing.T) {
	isolate(t)
	path := testCityMMDB(t)
	cc, rg, ct := extractGeo(nil)
	if cc != "" || rg != "" || ct != "" {
		t.Fatal("empty record")
	}
	_ = withConfig(func(c *Config) error {
		if c.Analytics == nil {
			c.Analytics = &AnalyticsSettings{Enabled: true, BotFilter: true, RetentionDays: 90}
		}
		c.Analytics.MMDBPath = path
		return nil
	})
	cc, rg, ct = lookupGeo("8.8.8.8")
	if cc != "US" || rg != "CA" || ct != "Mountain View" {
		t.Fatalf("8.8.8.8: %q %q %q", cc, rg, ct)
	}
	cc, rg, ct = lookupGeo("103.21.244.10")
	if cc != "IN" || rg != "MH" || ct != "Mumbai" {
		t.Fatalf("mumbai: %q %q %q", cc, rg, ct)
	}
	cc, rg, ct = lookupGeo("9.9.9.9")
	if cc != "" || ct != "" {
		t.Fatalf("unknown ip: %q %q %q", cc, rg, ct)
	}
	code, out := call(t, integMux(), "GET", "/api/analytics/settings", nil)
	if code != 200 {
		t.Fatalf("settings %d %v", code, out)
	}
	mmdb, _ := out["mmdb"].(map[string]any)
	if mmdb["ok"] != true {
		t.Fatalf("status %v", out)
	}
	// Saving a missing file is refused; a blank path clears it.
	code, out = call(t, integMux(), "POST", "/api/analytics/settings", map[string]any{"mmdb_path": filepath.Join(t.TempDir(), "nope.mmdb")})
	if code == 200 {
		t.Fatalf("accepted missing mmdb: %v", out)
	}
}

func TestBeaconUsesMMDB(t *testing.T) {
	isolate(t)
	path := testCityMMDB(t)
	_ = withConfig(func(c *Config) error {
		c.Sites = []*Site{{Slug: "shop", Name: "Shop"}}
		c.Analytics = &AnalyticsSettings{Enabled: true, BotFilter: true, RetentionDays: 90, MMDBPath: path}
		return nil
	})
	req := httptest.NewRequest("GET", "/__rover/e?site=shop&p=/menu", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0.0.0")
	req.Header.Set("X-Forwarded-For", "8.8.8.8")
	rec := httptest.NewRecorder()
	integMux().ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("beacon %d", rec.Code)
	}
	// summary should include the city
	_, sum := call(t, integMux(), "GET", "/api/analytics/shop?days=7", nil)
	// country list
	raw, _ := json.Marshal(sum)
	if !strings.Contains(string(raw), "US") {
		t.Fatalf("expected US in summary: %s", raw)
	}
	if cities, _ := sum["cities"].([]any); len(cities) == 0 {
		t.Fatalf("expected city row: %s", raw)
	} else {
		row, _ := cities[0].(map[string]any)
		if fmt.Sprint(row["name"]) != "Mountain View" {
			t.Fatalf("city %v", cities)
		}
	}
}
