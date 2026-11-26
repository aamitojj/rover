package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- versions ----------

func TestCompareVersionsPrerelease(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.6.0-beta.1", "0.6.0", -1}, {"0.6.0", "0.6.0-rc.1", 1}, {"0.6.0-beta.2", "0.6.0-beta.10", -1},
		{"v0.7.0-beta.1", "0.6.0", 1}, {"0.6.0-alpha", "0.6.0-beta", -1}, {"0.6.0-beta", "0.6.0-beta.1", -1},
		{"4.0.0-beta.420", "4.0.0-beta.435", -1}, {"0.6.0", "v0.6.0", 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compare(%s,%s)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNormalizeRepo(t *testing.T) {
	for in, want := range map[string]string{"aamitojj/NL2": "aamitojj/NL2", "https://github.com/amitogpandori/NL2.git": "amitogpandori/NL2", " github.com/a-b/c.d/ ": "a-b/c.d"} {
		got, err := normalizeRepo(in)
		if err != nil || got != want {
			t.Errorf("normalizeRepo(%q)=%q,%v want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "nouser", "a/b/c", "../x/y", "a/..", "a b/c"} {
		if _, err := normalizeRepo(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// ---------- mock GitHub ----------

type mockGH struct {
	srv      *httptest.Server
	mu       sync.Mutex
	releases []map[string]any // newest first, like GitHub
	files    map[string][]byte
	hits     map[string]int
}

func newMockGH(t *testing.T) *mockGH {
	m := &mockGH{files: map[string][]byte{}, hits: map[string]int{}}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.hits[r.URL.Path]++
		if r.Header.Get("User-Agent") == "" {
			http.Error(w, "no UA", 403)
			return
		}
		switch {
		case r.URL.Path == "/repos/o/r/releases/latest":
			for _, rel := range m.releases {
				if rel["draft"] != true && rel["prerelease"] != true {
					_ = json.NewEncoder(w).Encode(rel)
					return
				}
			}
			http.NotFound(w, r)
		case r.URL.Path == "/repos/o/r/releases":
			_ = json.NewEncoder(w).Encode(m.releases)
		case strings.HasPrefix(r.URL.Path, "/dl/"):
			b, ok := m.files[strings.TrimPrefix(r.URL.Path, "/dl/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(b)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(m.srv.Close)
	old := updateAPI
	updateAPI = m.srv.URL
	t.Cleanup(func() { updateAPI = old })
	return m
}

// release adds a release with the given files (name -> content) and a
// SHA256SUMS.txt (unless sums is non-nil, then that text is used).
func (m *mockGH) release(tag string, pre bool, files map[string]string, sums *string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var assets []map[string]any
	var lines []string
	for name, body := range files {
		key := tag + "/" + name
		m.files[key] = []byte(body)
		h := sha256.Sum256([]byte(body))
		assets = append(assets, map[string]any{"name": name, "size": len(body), "browser_download_url": m.srv.URL + "/dl/" + key, "digest": "sha256:" + hex.EncodeToString(h[:])})
		lines = append(lines, hex.EncodeToString(h[:])+"  "+name)
	}
	text := strings.Join(lines, "\n") + "\n"
	if sums != nil {
		text = *sums
	}
	if sums == nil || *sums != "" {
		key := tag + "/SHA256SUMS.txt"
		m.files[key] = []byte(text)
		assets = append(assets, map[string]any{"name": "SHA256SUMS.txt", "size": len(text), "browser_download_url": m.srv.URL + "/dl/" + key})
	}
	rel := map[string]any{"tag_name": tag, "name": "Rover " + tag, "body": "## What's new\n- Faster deploys", "draft": false, "prerelease": pre, "published_at": "2026-11-20T10:00:00Z", "html_url": "https://github.com/o/r/releases/tag/" + tag, "assets": assets}
	m.releases = append([]map[string]any{rel}, m.releases...)
}

func updateTestSetup(t *testing.T) *mockGH {
	t.Helper()
	isolate(t)
	oldV, oldOS, oldHook := Version, updateGOOS, installHook
	Version, updateGOOS = "0.6.0", "windows"
	t.Cleanup(func() { Version, updateGOOS, installHook = oldV, oldOS, oldHook })
	_ = withConfig(func(c *Config) error { c.Update = &UpdateSettings{Repo: "o/r"}; return nil })
	updState.Lock()
	updState.info, updState.notified, updState.auto = nil, "", ""
	updState.Unlock()
	return newMockGH(t)
}

func TestUpdateCheckNewerOlderSame(t *testing.T) {
	m := updateTestSetup(t)
	ctx := context.Background()
	m.release("v0.5.0", false, map[string]string{"rover.exe": "old"}, nil)
	if in := checkForUpdate(ctx); in.Newer || in.Latest != "0.5.0" || in.Error != "" {
		t.Fatalf("older release must not be offered: %+v", in)
	}
	m.release("v0.6.0", false, map[string]string{"rover.exe": "same"}, nil)
	if in := checkForUpdate(ctx); in.Newer || in.Latest != "0.6.0" {
		t.Fatalf("same version must not be offered: %+v", in)
	}
	m.release("v0.7.0", false, map[string]string{"rover.exe": "new", "Rover-Setup-0.7.0.exe": "setup"}, nil)
	in := checkForUpdate(ctx)
	if !in.Newer || in.Latest != "0.7.0" || in.Kind != "exe" || in.Asset != "rover.exe" || !strings.Contains(in.Notes, "Faster") {
		t.Fatalf("newer release: %+v", in)
	}
	if c := snapshotConfig(); c.Update.LastCheck == "" {
		t.Fatal("last check not stored")
	}
	// Installed copies (unins000.exe next to the program) get the installer.
	r := &ghRelease{Tag: "v0.7.0", Assets: []ghAsset{{Name: "rover.exe"}, {Name: "Rover-Setup-0.7.0.exe"}}}
	if a, k := pickAsset(r, "windows", true); a == nil || k != "installer" {
		t.Fatalf("installed copy should use the installer: %v %s", a, k)
	}
	if a, _ := pickAsset(r, "linux", false); a != nil {
		t.Fatal("no automatic install outside Windows")
	}
}

func TestUpdatePrereleaseOnlyOnBeta(t *testing.T) {
	m := updateTestSetup(t)
	m.release("v0.6.0", false, map[string]string{"rover.exe": "same"}, nil)
	m.release("v0.7.0-beta.1", true, map[string]string{"rover.exe": "beta"}, nil)
	m.mu.Lock()
	m.releases = append([]map[string]any{{"tag_name": "v9.9.9", "draft": true}}, m.releases...)
	m.mu.Unlock()
	if in := checkForUpdate(context.Background()); in.Newer {
		t.Fatalf("stable channel must ignore pre-releases and drafts: %+v", in)
	}
	_ = withConfig(func(c *Config) error { c.Update.Channel = "beta"; return nil })
	in := checkForUpdate(context.Background())
	if !in.Newer || in.Latest != "0.7.0-beta.1" || !in.Prerelease {
		t.Fatalf("beta channel should offer the pre-release: %+v", in)
	}
}

func TestUpdateInstallVerifiesChecksum(t *testing.T) {
	m := updateTestSetup(t)
	m.release("v0.7.0", false, map[string]string{"rover.exe": "new program"}, nil)
	var got string
	installHook = func(kind, path string) error {
		b, _ := os.ReadFile(path)
		got = kind + ":" + string(b)
		os.Remove(path)
		return nil
	}
	if err := updateJob("0.7.0")(&Job{ID: "t"}); err != nil {
		t.Fatal(err)
	}
	if got != "exe:new program" {
		t.Fatalf("installed %q", got)
	}
	// The user saw 0.7.0 but GitHub now offers 0.8.0: stop.
	m.release("v0.8.0", false, map[string]string{"rover.exe": "x"}, nil)
	if err := updateJob("0.7.0")(&Job{ID: "t"}); err == nil || !strings.Contains(err.Error(), "instead of") {
		t.Fatalf("expected version-changed error, got %v", err)
	}
}

func TestUpdateChecksumMismatchRejected(t *testing.T) {
	m := updateTestSetup(t)
	bad := strings.Repeat("ab", 32) + "  rover.exe\n"
	m.release("v0.7.0", false, map[string]string{"rover.exe": "tampered"}, &bad)
	// GitHub's digest would also disagree; drop it to test the file check alone.
	m.mu.Lock()
	for _, a := range m.releases[0]["assets"].([]map[string]any) {
		delete(a, "digest")
	}
	m.mu.Unlock()
	installHook = func(kind, path string) error { t.Fatal("must not install"); return nil }
	err := updateJob("0.7.0")(&Job{ID: "t"})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
	if exe, _ := os.Executable(); fileExists(exe + ".new") {
		t.Fatal("rejected download left behind")
	}
}

func TestUpdateDigestDisagreementAndMissingSums(t *testing.T) {
	m := updateTestSetup(t)
	installHook = func(kind, path string) error { t.Fatal("must not install"); return nil }
	sums := strings.Repeat("cd", 32) + "  rover.exe\n"
	m.release("v0.7.0", false, map[string]string{"rover.exe": "x"}, &sums) // digest is the real hash
	if err := updateJob("0.7.0")(&Job{ID: "t"}); err == nil || !strings.Contains(err.Error(), "disagree") {
		t.Fatalf("expected disagreement error, got %v", err)
	}
	none := ""
	m.release("v0.7.1", false, map[string]string{"rover.exe": "y"}, &none)
	if err := updateJob("0.7.1")(&Job{ID: "t"}); err == nil || !strings.Contains(err.Error(), "no SHA256SUMS.txt") {
		t.Fatalf("expected missing sums error, got %v", err)
	}
}

func TestUpdateSignature(t *testing.T) {
	m := updateTestSetup(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	old := updatePublicKey
	updatePublicKey = base64.StdEncoding.EncodeToString(pub)
	defer func() { updatePublicKey = old }()
	installHook = func(kind, path string) error { os.Remove(path); return nil }
	m.release("v0.7.0", false, map[string]string{"rover.exe": "signed"}, nil)
	if err := updateJob("0.7.0")(&Job{ID: "t"}); err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Fatalf("unsigned release must be refused when a key is built in: %v", err)
	}
	m.mu.Lock()
	sums := m.files["v0.7.0/SHA256SUMS.txt"]
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, sums))
	m.files["v0.7.0/SHA256SUMS.txt.sig"] = []byte(sig + "\n")
	assets := m.releases[0]["assets"].([]map[string]any)
	m.releases[0]["assets"] = append(assets, map[string]any{"name": "SHA256SUMS.txt.sig", "browser_download_url": m.srv.URL + "/dl/v0.7.0/SHA256SUMS.txt.sig"})
	m.mu.Unlock()
	if err := updateJob("0.7.0")(&Job{ID: "t"}); err != nil {
		t.Fatalf("signed release refused: %v", err)
	}
	m.mu.Lock()
	m.files["v0.7.0/SHA256SUMS.txt.sig"] = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte("other"))))
	m.mu.Unlock()
	if err := updateJob("0.7.0")(&Job{ID: "t"}); err == nil || !strings.Contains(err.Error(), "signature is not valid") {
		t.Fatalf("bad signature accepted: %v", err)
	}
}

func TestUpdateOfflineAndNoReleases(t *testing.T) {
	updateTestSetup(t)
	in := checkForUpdate(context.Background())
	if !in.NoReleases || in.Newer || in.Error != "" {
		t.Fatalf("empty repo: %+v", in)
	}
	updateAPI = "http://127.0.0.1:1" // nothing listens here
	in = checkForUpdate(context.Background())
	if !in.Offline || in.Newer || in.Error == "" {
		t.Fatalf("offline should be reported calmly: %+v", in)
	}
	if err := updateJob("0.7.0")(&Job{ID: "t"}); err == nil || !strings.Contains(err.Error(), "internet") {
		t.Fatalf("offline install: %v", err)
	}
}

func TestUpdateDownloadHostsRestricted(t *testing.T) {
	for _, ok := range []string{"https://github.com/o/r/releases/download/v1/x.exe", "https://objects.githubusercontent.com/x", "https://release-assets.githubusercontent.com/x"} {
		if err := allowedDownloadURL(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"http://github.com/x", "https://evil.example/x", "https://github.com.evil.example/x", "ftp://github.com/x"} {
		if err := allowedDownloadURL(bad); err == nil {
			t.Errorf("%s should be refused", bad)
		}
	}
}

func TestUpdateRoutesSkipLaterSettings(t *testing.T) {
	m := updateTestSetup(t)
	m.release("v0.7.0", false, map[string]string{"rover.exe": "n"}, nil)
	mux := http.NewServeMux()
	updateRoutes(mux)
	code, out := call(t, mux, "GET", "/api/update/check", nil)
	if code != 200 || out["newer"] != true {
		t.Fatalf("check: %d %v", code, out)
	}
	if code, _ = call(t, mux, "POST", "/api/update/skip", map[string]any{"version": "0.7.0"}); code != 200 {
		t.Fatal(code)
	}
	if _, out = call(t, mux, "GET", "/api/update/check", nil); out["skipped"] != true {
		t.Fatalf("skip not applied: %v", out)
	}
	call(t, mux, "POST", "/api/update/later", map[string]any{"hours": 24})
	m.release("v0.8.0", false, map[string]string{"rover.exe": "n2"}, nil)
	if _, out = call(t, mux, "GET", "/api/update/check", nil); out["skipped"] == true || out["snoozed"] != true {
		t.Fatalf("a newer version is not skipped but is snoozed: %v", out)
	}
	if code, out = call(t, mux, "POST", "/api/update/settings", map[string]any{"repo": "not a repo"}); code != 400 {
		t.Fatalf("bad repo accepted: %d %v", code, out)
	}
	code, out = call(t, mux, "POST", "/api/update/settings", map[string]any{"repo": "https://github.com/amitogpandori/NL2", "auto_check": false, "install": "auto", "channel": "beta"})
	s, _ := out["settings"].(map[string]any)
	if code != 200 || s["repo"] != "amitogpandori/NL2" || s["install"] != "auto" || s["channel"] != "beta" || s["auto_check"] != false {
		t.Fatalf("settings: %d %v", code, out)
	}
	_, out = call(t, mux, "GET", "/api/update/status", nil)
	if out["current"] != "0.6.0" || out["auto_check"] != false {
		t.Fatalf("status: %v", out)
	}
}

func TestUpdaterTickNotifiesOnceAndRespectsToggle(t *testing.T) {
	m := updateTestSetup(t)
	m.release("v0.7.0", false, map[string]string{"rover.exe": "n"}, nil)
	now := time.Now()
	updaterTick(context.Background(), now)
	updaterTick(context.Background(), now.Add(25*time.Hour))
	n := 0
	for _, x := range snapshotConfig().Notices {
		if strings.Contains(x.Text, "0.7.0 is available") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want exactly one notice, got %d", n)
	}
	off := false
	_ = withConfig(func(c *Config) error { c.Update.AutoCheck = &off; c.Update.LastCheck = ""; return nil })
	before := m.hits["/repos/o/r/releases/latest"]
	updaterTick(context.Background(), now.Add(50*time.Hour))
	if m.hits["/repos/o/r/releases/latest"] != before {
		t.Fatal("automatic check ran although it is turned off")
	}
}

// ---------- project hub ----------

func TestApr1MatchesOpenSSL(t *testing.T) {
	if got := apr1("S3cret-pass", "abcdefgh"); got != "$apr1$abcdefgh$Dhp923kOcdRj9IQgweX30/" {
		t.Fatalf("apr1 = %s", got)
	}
	if got := apr1("a", "xY12"); got != "$apr1$xY12$.kAvv76AESGkMK9/elMDP1" {
		t.Fatalf("apr1 short = %s", got)
	}
	if s := newSalt(); len(s) != 8 || strings.ContainsAny(s, "$:") {
		t.Fatalf("salt %q", s)
	}
}

func TestSiteConfExtras(t *testing.T) {
	isolate(t)
	base := generateNginxConf(nil, &siteFiles{})
	if siteConfExtras(base, &Site{}) != base {
		t.Fatal("no settings must leave the config unchanged")
	}
	h, _ := protectString(apr1("password1", "saltsalt"))
	s := &Site{Domain: "bakery.in", IncludeWWW: true, Primary: "apex", HTTPSActive: true, Access: &SiteAccess{User: "team", Hash: h}}
	out := siteConfExtras(base, s)
	for _, want := range []string{"auth_basic \"Private site\";", "auth_basic_user_file /etc/nginx/conf.d/.htpasswd;", "if ($host = www.bakery.in) { return 301 https://bakery.in$request_uri; }"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if !strings.Contains(htpasswdScript(s), "team:$apr1$saltsalt$") {
		t.Fatalf("htpasswd script: %s", htpasswdScript(s))
	}
	if !strings.Contains(htpasswdScript(&Site{}), `rm -f "$BASE/nginx/.htpasswd"`) {
		t.Fatal("turning protection off must remove the file")
	}
	s.Primary = "www"
	if !strings.Contains(siteConfExtras(base, s), "if ($host = bakery.in) { return 301 https://www.bakery.in$request_uri; }") {
		t.Fatal("www primary")
	}
}

func TestIncrementalPlan(t *testing.T) {
	old := map[string]string{"index.html": "a", "app.js": "b", "gone.css": "c"}
	cur := map[string]string{"index.html": "a", "app.js": "B", "new.png": "d"}
	ch, del := diffManifest(old, cur)
	if strings.Join(ch, ",") != "app.js,new.png" || strings.Join(del, ",") != "gone.css" {
		t.Fatalf("changed %v deleted %v", ch, del)
	}
	pre := incPrelude(&incPlan{Prev: "20261120-101010", Deleted: del})
	for _, want := range []string{`PREV="$BASE/releases/20261120-101010"`, `cp -a "$PREV/." "$REL/"`, "gone.css", "exit 42"} {
		if !strings.Contains(pre, want) {
			t.Errorf("prelude missing %q:\n%s", want, pre)
		}
	}
	if incPrelude(nil) != "" {
		t.Fatal("full deploys have no prelude")
	}
	for _, bad := range []string{"../x", "a\nb", "/etc/passwd", ""} {
		if safeManifestPath(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
	isolate(t)
	saveManifest("shop", &fileManifest{Release: "r1", Files: cur})
	if m := loadManifest("shop"); m == nil || m.Release != "r1" || m.Files["app.js"] != "B" {
		t.Fatalf("manifest round trip: %+v", m)
	}
}

func hubSetup(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	isolate(t)
	_ = withConfig(func(c *Config) error {
		c.Sites = append(c.Sites, &Site{Slug: "shop", Name: "Shop", Domain: "shop.in", IncludeWWW: true})
		return nil
	})
	ensureProjects()
	dir, _, err := projectDir("shop")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	project06Routes(mux)
	return mux, dir
}

func TestProjectConfigAPI(t *testing.T) {
	mux, _ := hubSetup(t)
	code, out := call(t, mux, "POST", "/api/sites/shop/config", map[string]any{"build_cmd": "rm -rf /"})
	if code != 400 {
		t.Fatalf("unsafe build command accepted: %v", out)
	}
	if code, _ = call(t, mux, "POST", "/api/sites/shop/config", map[string]any{"publish_dir": "../outside"}); code != 400 {
		t.Fatal("publish folder outside the project accepted")
	}
	code, out = call(t, mux, "POST", "/api/sites/shop/config", map[string]any{"locked": true, "build_cmd": "npm  run  build", "publish_dir": `.\dist\`, "primary": "www", "alert_down": false, "no_backup": true, "incremental": false})
	if code != 200 || out["locked"] != true || out["build_cmd"] != "npm run build" || out["publish_dir"] != "dist" || out["primary"] != "www" || out["alert_down"] != false || out["alert_fail"] != true || out["incremental"] != false {
		t.Fatalf("config: %d %v", code, out)
	}
	c := snapshotConfig()
	if s := findSite(&c, "shop"); !s.Locked || !s.NoBackup || !c.NoIncremental {
		t.Fatalf("not stored: %+v", s)
	}
	if code, _ = call(t, mux, "POST", "/api/sites/shop/access", map[string]any{"enabled": true, "user": "team", "password": "short"}); code != 400 {
		t.Fatal("short password accepted")
	}
	code, out = call(t, mux, "POST", "/api/sites/shop/access", map[string]any{"enabled": true, "user": "team", "password": "long enough pw"})
	if code != 200 || out["access_on"] != true {
		t.Fatalf("access: %v", out)
	}
	c = snapshotConfig()
	s := findSite(&c, "shop")
	if s.Access == nil || strings.Contains(s.Access.Hash, "long enough") || !strings.Contains(htpasswdFor(s), "team:$apr1$") {
		t.Fatalf("password must be stored only as a hash: %+v", s.Access)
	}
	call(t, mux, "POST", "/api/sites/shop/access", map[string]any{"enabled": false})
	if c = snapshotConfig(); findSite(&c, "shop").Access != nil {
		t.Fatal("protection not removed")
	}
}

func TestRulesEditorAPI(t *testing.T) {
	mux, dir := hubSetup(t)
	cur := filepath.Join(dir, "current", "dist")
	_ = os.MkdirAll(cur, 0o755)
	_ = os.WriteFile(filepath.Join(cur, "index.html"), []byte("<h1>hi</h1>"), 0o644)
	_ = os.WriteFile(filepath.Join(cur, "_redirects"), []byte("/old /new 301\n"), 0o644)
	code, out := call(t, mux, "GET", "/api/projects/shop/rules", nil)
	if code != 200 || out["publish"] != "dist" || !strings.Contains(fmt.Sprint(out["redirects"]), "/old /new") {
		t.Fatalf("rules: %d %v", code, out)
	}
	code, out = call(t, mux, "POST", "/api/projects/shop/rules", map[string]any{"redirects": "/a\n", "headers": ""})
	if code != 200 || out["saved"] != false || out["fails"].(float64) < 1 {
		t.Fatalf("broken rule must not be saved: %v", out)
	}
	code, out = call(t, mux, "POST", "/api/projects/shop/rules", map[string]any{"redirects": "/shop/* /store/:splat 301", "headers": "/*\n  X-Frame-Options: DENY"})
	if code != 200 || out["saved"] != true {
		t.Fatalf("save: %v", out)
	}
	b, _ := os.ReadFile(filepath.Join(cur, "_headers"))
	if !strings.Contains(string(b), "X-Frame-Options: DENY") {
		t.Fatalf("headers file: %q", b)
	}
	call(t, mux, "POST", "/api/projects/shop/rules", map[string]any{"redirects": "", "headers": ""})
	if fileExists(filepath.Join(cur, "_redirects")) {
		t.Fatal("emptying the editor should remove the file")
	}
}

func TestDraftDeployKeepsLiveFiles(t *testing.T) {
	_, dir := hubSetup(t)
	src := writeTree(t, map[string]string{"index.html": "<h1>v2</h1>", "about.html": "x"})
	j := &Job{ID: "t"}
	if err := deployLocal(j, "shop", src, true); err != nil {
		t.Fatal(err)
	}
	ix, _ := readIndex(dir)
	if len(ix.Deploys) != 1 || ix.Deploys[0].Status != "draft" {
		t.Fatalf("want one draft: %+v", ix.Deploys)
	}
	if fileExists(filepath.Join(dir, "current", "index.html")) {
		t.Fatal("a draft must not replace the live files on this PC")
	}
	// Locked projects turn automatic deploys into drafts too (no server needed).
	_ = withConfig(func(c *Config) error { findSite(c, "shop").Locked = true; return nil })
	ws := loadWS(t, map[string]string{"index.html": "<h1>v3</h1>"})
	ws.auto = true
	if err := deployWorkspace(&Job{ID: "t2"}, "shop", ws); err != nil {
		t.Fatalf("locked auto deploy should be saved as a draft: %v", err)
	}
	if ix, _ = readIndex(dir); len(ix.Deploys) != 2 || ix.Deploys[0].Status != "draft" {
		t.Fatalf("locked auto deploy: %+v", ix.Deploys)
	}
}

func TestQuickDeployAndRecentFolders(t *testing.T) {
	mux, _ := hubSetup(t)
	setQuickDeploy(`"C:\Sites\shop\dist"`)
	_, out := call(t, mux, "GET", "/api/quick", nil)
	if out["pending"] != `C:\Sites\shop\dist` {
		t.Fatalf("pending: %v", out)
	}
	call(t, mux, "POST", "/api/quick/clear", nil)
	if _, out = call(t, mux, "GET", "/api/quick", nil); out["pending"] != "" {
		t.Fatalf("not cleared: %v", out)
	}
	for i := 0; i < 10; i++ {
		rememberFolder(fmt.Sprintf("/sites/%d", i))
	}
	rememberFolder("/sites/3")
	r := snapshotConfig().RecentFolders
	if len(r) != 8 || r[0] != "/sites/3" || r[1] != "/sites/9" {
		t.Fatalf("recent: %v", r)
	}
	if code, out := call(t, mux, "POST", "/api/sites/shop/redeploy-last", map[string]any{}); code != 400 || !strings.Contains(fmt.Sprint(out["error"]), "does not know a folder") {
		t.Fatalf("redeploy without a folder: %d %v", code, out)
	}
}

func TestMigrateV06(t *testing.T) {
	isolate(t)
	c := &Config{ConfigVersion: 5, UpdateManifestURL: "https://example.com/latest.json"}
	if !migrateConfig(c) || c.ConfigVersion != currentConfigVersion || c.UpdateManifestURL != "" {
		t.Fatalf("migration: %+v", c)
	}
	u := updateSettings(c)
	if u.Repo != defaultUpdateRepo || u.Install != "ask" || u.Channel != "stable" || !autoCheckOn(u) {
		t.Fatalf("defaults: %+v", u)
	}
}

// TestReleaseScriptIncremental runs the real server script with bash in a
// temporary folder: a full release, then a changed-files-only release.
func TestReleaseScriptIncremental(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil || runtime.GOOS == "windows" {
		t.Skip("needs bash")
	}
	isolate(t)
	base := t.TempDir()
	run := func(release string, files map[string]string, inc *incPlan, site *Site) (string, error) {
		var entries []srcEntry
		var total int64
		for p, body := range files {
			b := body
			entries = append(entries, srcEntry{Path: p, Size: int64(len(b)), Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(b)), nil }})
			total += int64(len(b))
		}
		tgz, _, _, err := writeTarball(entries, total)
		if err != nil {
			t.Fatal(err)
		}
		script := releaseScript(base, tgz, release, site, generateNginxConf(nil, &siteFiles{}), inc)
		out, err := exec.Command("bash", "-c", script).CombinedOutput()
		return string(out), err
	}
	site := &Site{Slug: "shop", AppUUID: "nope-" + randHex(4)}
	if out, err := run("20261120-100000", map[string]string{"index.html": "home", "a.js": "v1", "old/gone.css": "x"}, nil, site); err != nil {
		t.Fatalf("full release: %v\n%s", err, out)
	}
	h, _ := protectString(apr1("password1", "saltsalt"))
	site.Access = &SiteAccess{User: "team", Hash: h}
	out, err := run("20261120-100100", map[string]string{"a.js": "v2", "img/new.png": "png"}, &incPlan{Prev: "20261120-100000", Deleted: []string{"old/gone.css"}}, site)
	if err != nil {
		t.Fatalf("incremental release: %v\n%s", err, out)
	}
	live := filepath.Join(base, "html")
	read := func(p string) string { b, _ := os.ReadFile(filepath.Join(live, p)); return string(b) }
	if read("index.html") != "home" || read("a.js") != "v2" || read("img/new.png") != "png" || fileExists(filepath.Join(live, "old/gone.css")) {
		t.Fatalf("incremental result wrong: index=%q a=%q png=%q", read("index.html"), read("a.js"), read("img/new.png"))
	}
	if b, _ := os.ReadFile(filepath.Join(base, "nginx", ".htpasswd")); !strings.HasPrefix(string(b), "team:$apr1$") {
		t.Fatalf(".htpasswd not written: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(base, "nginx", "default.conf")); !strings.Contains(string(b), "auth_basic_user_file") {
		t.Fatal("live config lacks password protection")
	}
	// The previous release is gone or not live: exit 42 and nothing changes.
	out, err = run("20261120-100200", map[string]string{"a.js": "v3"}, &incPlan{Prev: "20261120-100000"}, site)
	if err == nil || !strings.Contains(out, "HP_NEED_FULL") {
		t.Fatalf("expected a request for a full upload: %v\n%s", err, out)
	}
	if read("a.js") != "v2" || fileExists(filepath.Join(base, "releases", "20261120-100200", "a.js")) {
		t.Fatal("a refused fast deploy must not change anything")
	}
	site.Access = nil
	if out, err := run("20261120-100300", map[string]string{"index.html": "home"}, nil, site); err != nil || fileExists(filepath.Join(base, "nginx", ".htpasswd")) {
		t.Fatalf("turning protection off: %v %s", err, out)
	}
}
