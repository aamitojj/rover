package main

// Rover 0.7: "Host on this PC".
//
// Rover's own web server serves the projects that have This PC as one of
// their servers, with the same Netlify rules (_redirects, _headers,
// netlify.toml), password protection and primary-domain redirect as the
// nginx config on real servers. Visitors reach it through a Cloudflare
// tunnel (cloudflared, downloaded on demand and checked against a pinned
// SHA-256), so no router changes are needed and it also works on phone
// mobile data.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const thisPCID = "this-pc"

// ThisPCConfig: hosting on this computer.
type ThisPCConfig struct {
	Enabled          bool `json:"enabled"`
	Port             int  `json:"port,omitempty"`               // built-in web server (default 8080)
	LAN              bool `json:"lan,omitempty"`                // also answer other devices at home
	StartWithWindows bool `json:"start_with_windows,omitempty"` //
	PreventSleep     bool `json:"prevent_sleep,omitempty"`      // keep the PC awake while hosting
}

func thisPCPort(c *Config) int {
	if c.ThisPC != nil && c.ThisPC.Port > 0 {
		return c.ThisPC.Port
	}
	return 8080
}

func thisPCOn(c *Config) bool { return c.ThisPC != nil && c.ThisPC.Enabled }

// hostingRoot is where This PC keeps the published versions.
var hostingRoot = func() string {
	d, err := configDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "hosting")
}

var slugSafe = func(slug string) bool { return slug != "" && !strings.ContainsAny(slug, `/\.:`) }

func localSiteDir(slug string) string { return filepath.Join(hostingRoot(), slug) }

// ---------- local releases ----------

// lastRules carries the parsed rules of the deploy being published, so This
// PC can apply them (they are not part of the upload).
var lastRules sync.Map // slug -> *SiteRules

func rememberRules(slug string, r *SiteRules) {
	if r != nil {
		lastRules.Store(slug, r)
	}
}

type savedRules struct {
	Redirects []*RedirectRule `json:"redirects"`
	Headers   []*HeaderRule   `json:"headers"`
}

func loadSavedRules(dir string) *SiteRules {
	s := &SiteRules{}
	b, err := os.ReadFile(filepath.Join(dir, ".dilreet-rules.json"))
	if err != nil {
		return s
	}
	var sr savedRules
	if json.Unmarshal(b, &sr) != nil {
		return s
	}
	for _, r := range sr.Redirects {
		s.addRedirect(r)
	}
	for _, h := range sr.Headers {
		s.addHeader(h)
	}
	return s
}

// extractTarGz unpacks a site upload, refusing anything outside dst.
func extractTarGz(src, dst string) (int, error) {
	f, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	tr := tar.NewReader(gz)
	n := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		name := filepath.FromSlash(strings.TrimPrefix(h.Name, "./"))
		if name == "" || name == "." {
			continue
		}
		target := filepath.Join(dst, name)
		if rel, err := filepath.Rel(dst, target); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(name) {
			return n, fmt.Errorf("unsafe path in upload: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return n, err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return n, err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return n, err
			}
			if _, err := io.Copy(out, io.LimitReader(tr, 2<<30)); err != nil {
				out.Close()
				return n, err
			}
			out.Close()
			n++
		}
	}
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp-" + randHex(3)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// localPublish makes a release live on This PC (same release name as the
// other servers). The switch is a single file rename.
var localPublish = func(slug, tarball, release string, resync bool) error {
	if !slugSafe(slug) || !releaseRe.MatchString(release) {
		return errors.New("invalid project or release name")
	}
	base := localSiteDir(slug)
	rel := filepath.Join(base, "releases", release)
	if resync && localLiveRelease(slug) == release {
		if _, err := os.Stat(rel); err == nil {
			return nil
		}
	}
	tmp := rel + ".part"
	os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	if _, err := extractTarGz(tarball, tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if v, ok := lastRules.Load(slug); ok {
		r := v.(*SiteRules)
		b, _ := json.Marshal(savedRules{Redirects: r.Redirects, Headers: r.Headers})
		_ = os.WriteFile(filepath.Join(tmp, ".dilreet-rules.json"), b, 0o644)
	}
	os.RemoveAll(rel)
	if err := os.Rename(tmp, rel); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(base, "live.txt"), []byte(release)); err != nil {
		return err
	}
	pruneLocalReleases(base, release, 5)
	liveSites.forget(slug)
	return nil
}

func pruneLocalReleases(base, keep string, n int) {
	ents, err := os.ReadDir(filepath.Join(base, "releases"))
	if err != nil {
		return
	}
	var names []string
	for _, e := range ents {
		if e.IsDir() && releaseRe.MatchString(e.Name()) && e.Name() != "initial" {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for i, name := range names {
		if i >= n && name != keep {
			os.RemoveAll(filepath.Join(base, "releases", name))
		}
	}
}

func localActivate(slug, release string) error {
	if !slugSafe(slug) || !releaseRe.MatchString(release) {
		return errors.New("invalid release")
	}
	base := localSiteDir(slug)
	if st, err := os.Stat(filepath.Join(base, "releases", release)); err != nil || !st.IsDir() {
		return fmt.Errorf("release %s is not on This PC", release)
	}
	liveSites.forget(slug)
	return writeFileAtomic(filepath.Join(base, "live.txt"), []byte(release))
}

func localLiveRelease(slug string) string {
	b, err := os.ReadFile(filepath.Join(localSiteDir(slug), "live.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func localEnsureSite(slug string) error {
	if !slugSafe(slug) {
		return errors.New("invalid project id")
	}
	base := localSiteDir(slug)
	if localLiveRelease(slug) != "" {
		return nil
	}
	dir := filepath.Join(base, "releases", "initial")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(placeholderHTML), 0o644); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(base, "live.txt"), []byte("initial"))
}

// ---------- the built-in web server ----------

type dirFiles struct{ root string }

func (d dirFiles) IsFile(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, ".dilreet") {
		return false
	}
	st, err := os.Stat(filepath.Join(d.root, filepath.FromSlash(rel)))
	return err == nil && st.Mode().IsRegular()
}

type liveSite struct {
	release string
	dir     string
	rules   *SiteRules
}

type liveCache struct {
	sync.Mutex
	m map[string]*liveSite
}

var liveSites = &liveCache{m: map[string]*liveSite{}}

func (lc *liveCache) forget(slug string) {
	lc.Lock()
	delete(lc.m, slug)
	lc.Unlock()
}

func liveOf(slug string) *liveSite {
	rel := localLiveRelease(slug)
	if rel == "" {
		return nil
	}
	liveSites.Lock()
	defer liveSites.Unlock()
	if ls := liveSites.m[slug]; ls != nil && ls.release == rel {
		return ls
	}
	dir := filepath.Join(localSiteDir(slug), "releases", rel)
	ls := &liveSite{release: rel, dir: dir, rules: loadSavedRules(dir)}
	liveSites.m[slug] = ls
	return ls
}

// LocalHost serves the projects hosted on this PC.
type LocalHost struct{ cfg func() Config }

func (lh LocalHost) siteFor(host string) *Site {
	c := lh.cfg()
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	var mine []*Site
	for _, s := range c.Sites {
		on := false
		for _, id := range siteServerIDs(s) {
			if id == thisPCID {
				on = true
			}
		}
		if !on {
			continue
		}
		mine = append(mine, s)
		if host == s.Slug+".localhost" || host == s.Slug+".local" {
			return s
		}
		if s.Domain != "" {
			for _, h := range siteHosts(s) {
				if host == h {
					return s
				}
			}
		}
	}
	if len(mine) == 1 && (net.ParseIP(host) != nil || host == "localhost") {
		return mine[0] // only one project here: http://<this PC's address>:port works too
	}
	return nil
}

func (lh LocalHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s := lh.siteFor(r.Host)
	if s == nil {
		http.Error(w, "No Rover project here answers to "+r.Host, http.StatusNotFound)
		return
	}
	ls := liveOf(s.Slug)
	if ls == nil {
		http.Error(w, "This project has not been published on this PC yet.", http.StatusServiceUnavailable)
		return
	}
	if hp := htpasswdFor(s); hp != "" && !basicAuthOK(r, hp) {
		w.Header().Set("WWW-Authenticate", `Basic realm="Private site"`)
		http.Error(w, "401 password required", http.StatusUnauthorized)
		return
	}
	if to := primaryRedirect(s, r.Host); to != "" {
		http.Redirect(w, r, to+r.URL.RequestURI(), http.StatusMovedPermanently)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p, ok := cleanURLPath(r.URL.Path)
	if !ok {
		http.Error(w, "400 bad request", http.StatusBadRequest)
		return
	}
	fs := dirFiles{ls.dir}
	res := ls.rules.resolve(fs, p, r.URL.RawQuery)
	h := w.Header()
	h.Set("Cache-Control", defaultCacheControl)
	for _, kv := range ls.rules.headersFor(p) {
		h.Set(http.CanonicalHeaderKey(kv[0]), kv[1])
	}
	if res.Location != "" {
		h.Set("Location", res.Location)
		h.Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(res.Status)
		if r.Method == http.MethodGet {
			fmt.Fprintf(w, "Redirecting to %s\n", res.Location)
		}
		return
	}
	if res.File == "" {
		h.Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(res.Status)
		fmt.Fprintf(w, "<!doctype html><title>%d</title><p>%d Not Found</p>", res.Status, res.Status)
		return
	}
	f, err := os.Open(filepath.Join(ls.dir, filepath.FromSlash(res.File)))
	if err != nil {
		http.Error(w, "500 cannot read file", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	h.Set("Content-Type", contentTypeFor(res.File))
	h.Set("X-Content-Type-Options", "nosniff")
	if res.Status == http.StatusOK {
		http.ServeContent(w, r, res.File, st.ModTime(), f)
		return
	}
	h.Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	w.WriteHeader(res.Status)
	if r.Method == http.MethodGet {
		io.Copy(w, f)
	}
}

// primaryRedirect mirrors the www <-> bare-domain redirect of the nginx config.
func primaryRedirect(s *Site, host string) string {
	if s.Domain == "" || !s.IncludeWWW || (s.Primary != "apex" && s.Primary != "www") {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	from, to := "www."+s.Domain, s.Domain
	if s.Primary == "www" {
		from, to = s.Domain, "www."+s.Domain
	}
	if !strings.EqualFold(host, from) {
		return ""
	}
	return "https://" + to
}

// basicAuthOK checks a request against .htpasswd content (apr1 hashes).
func basicAuthOK(r *http.Request, htpasswd string) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Basic ") {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
	if err != nil {
		return false
	}
	user, pass, ok := strings.Cut(string(raw), ":")
	if !ok {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(htpasswd), "\n") {
		u, hash, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || u != user || !strings.HasPrefix(hash, "$apr1$") {
			continue
		}
		parts := strings.Split(hash, "$") // "", apr1, salt, digest
		if len(parts) < 4 {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(apr1(pass, parts[2])), []byte(hash)) == 1
	}
	return false
}

var localSrv = struct {
	sync.Mutex
	srv  *http.Server
	addr string
	err  string
}{}

// applyThisPC starts, restarts or stops the built-in web server and the
// Windows helpers to match the settings.
func applyThisPC() {
	c := snapshotConfig()
	localSrv.Lock()
	defer localSrv.Unlock()
	want := ""
	if thisPCOn(&c) {
		host := "127.0.0.1"
		if c.ThisPC.LAN {
			host = "0.0.0.0"
		}
		want = net.JoinHostPort(host, strconv.Itoa(thisPCPort(&c)))
	}
	if localSrv.srv != nil && localSrv.addr != want {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = localSrv.srv.Shutdown(ctx)
		cancel()
		localSrv.srv, localSrv.addr = nil, ""
	}
	if want != "" && localSrv.srv == nil {
		ln, err := net.Listen("tcp", want)
		if err != nil {
			localSrv.err = fmt.Sprintf("port %d is in use by another program: choose another port", thisPCPort(&c))
		} else {
			localSrv.err = ""
			localSrv.srv = &http.Server{Handler: LocalHost{cfg: snapshotConfig}, ReadHeaderTimeout: 20 * time.Second, IdleTimeout: 90 * time.Second}
			localSrv.addr = want
			go localSrv.srv.Serve(ln)
		}
	}
	on := thisPCOn(&c)
	setKeepAwake(on && c.ThisPC.PreventSleep)
	if c.ThisPC != nil {
		_ = setAutostart(c.ThisPC.StartWithWindows)
	}
	if on {
		if p := findServer(&c, thisPCID); p != nil && p.TunnelID != "" && p.TunnelToken != "" {
			go func() { _ = localTunnels.start(p.TunnelID, secret(p.TunnelToken)) }()
		}
	} else {
		localTunnels.stopAll()
	}
}

// ensureThisPCProfile adds This PC to the server list.
func ensureThisPCProfile(c *Config) {
	if findServer(c, thisPCID) != nil {
		return
	}
	c.Servers = append(c.Servers, &ServerProfile{ID: thisPCID, Label: "This PC", Provider: "thispc", Kind: "thispc", ServerIP: "127.0.0.1", Added: time.Now().Format(time.RFC3339)})
}

// ---------- cloudflared on this PC ----------

type tunnelProc struct {
	cancel  context.CancelFunc
	started time.Time
	lastErr string
	running bool
}

var localTunnels = &tunnelSet{m: map[string]*tunnelProc{}}

type tunnelSet struct {
	sync.Mutex
	m map[string]*tunnelProc
}

func cloudflaredPath() string {
	d, _ := configDir()
	name := "cloudflared"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(d, "bin", name)
}

// ensureCloudflared downloads the pinned cloudflared once and checks it.
var ensureCloudflared = func() (string, error) {
	path := cloudflaredPath()
	asset := osAsset()
	want := cloudflaredSHA256[asset]
	if want == "" {
		return "", fmt.Errorf("no cloudflared build is pinned for %s", asset)
	}
	if sum, err := fileSHA256(path); err == nil && sum == want {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	cl := &http.Client{Timeout: 5 * time.Minute}
	resp, err := cl.Get(cloudflaredURL(asset))
	if err != nil {
		return "", fmt.Errorf("download cloudflared: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download cloudflared: HTTP %d", resp.StatusCode)
	}
	tmp := path + ".download"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 200<<20))
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		os.Remove(tmp)
		return "", fmt.Errorf("cloudflared checksum mismatch (got %s); the download was rejected", got[:16])
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// start runs cloudflared for a tunnel and restarts it if it stops.
func (ts *tunnelSet) start(id, token string) error {
	ts.Lock()
	if p := ts.m[id]; p != nil && p.running {
		ts.Unlock()
		return nil
	}
	ts.Unlock()
	bin, err := ensureCloudflared()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	tp := &tunnelProc{cancel: cancel, started: time.Now(), running: true}
	ts.Lock()
	ts.m[id] = tp
	ts.Unlock()
	go func() {
		backoff := 5 * time.Second
		for ctx.Err() == nil {
			cmd := exec.CommandContext(ctx, bin, "tunnel", "--no-autoupdate", "run")
			cmd.Env = append(os.Environ(), "TUNNEL_TOKEN="+token)
			hideConsole(cmd)
			start := time.Now()
			err := cmd.Run()
			ts.Lock()
			if err != nil && ctx.Err() == nil {
				tp.lastErr = shortErr(err)
			}
			ts.Unlock()
			if time.Since(start) > time.Minute {
				backoff = 5 * time.Second
			}
			select {
			case <-ctx.Done():
			case <-time.After(backoff):
			}
			if backoff < 2*time.Minute {
				backoff *= 2
			}
		}
		ts.Lock()
		tp.running = false
		ts.Unlock()
	}()
	return nil
}

func (ts *tunnelSet) stop(id string) {
	ts.Lock()
	defer ts.Unlock()
	if p := ts.m[id]; p != nil {
		p.cancel()
		delete(ts.m, id)
	}
}

func (ts *tunnelSet) stopAll() {
	ts.Lock()
	defer ts.Unlock()
	for id, p := range ts.m {
		p.cancel()
		delete(ts.m, id)
	}
}

func (ts *tunnelSet) status() map[string]any {
	ts.Lock()
	defer ts.Unlock()
	out := map[string]any{}
	for id, p := range ts.m {
		out[id] = map[string]any{"running": p.running, "since": p.started.Format(time.RFC3339), "error": p.lastErr}
	}
	return out
}

// ---------- network monitor ----------

// netAdapter is one network connection of this PC.
type netAdapter struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"` // wifi | ethernet | usb-tether | mobile | other
	Up      bool     `json:"up"`
	Gateway bool     `json:"gateway"` // has a default gateway (can reach the internet)
	Metric  int      `json:"metric"`
	IPv4    []string `json:"ipv4,omitempty"`
	SSID    string   `json:"ssid,omitempty"`
}

// activeAdapter is the connection Windows uses for the internet (lowest
// metric among adapters with a gateway).
func activeAdapter(list []netAdapter) *netAdapter {
	var best *netAdapter
	for i := range list {
		a := &list[i]
		if !a.Up || !a.Gateway {
			continue
		}
		if best == nil || a.Metric < best.Metric {
			best = a
		}
	}
	return best
}

func kindLabel(k string) string {
	return map[string]string{"wifi": "Wi-Fi", "ethernet": "cable (Ethernet)", "usb-tether": "phone over USB", "mobile": "mobile data", "other": "other connection"}[k]
}

var netMon = struct {
	sync.Mutex
	last    time.Time
	active  string // "kind:name"
	online  bool
	known   bool
	offline time.Time
	history []FailoverEvent
}{}

func netMonEvent(text string) {
	netMon.history = append([]FailoverEvent{{Time: time.Now().Format(time.RFC3339), Text: text}}, netMon.history...)
	if len(netMon.history) > 20 {
		netMon.history = netMon.history[:20]
	}
}

// netMonitorTick watches the PC's internet while it hosts sites.
func netMonitorTick(now time.Time) {
	c := snapshotConfig()
	if !thisPCOn(&c) {
		return
	}
	netMon.Lock()
	if now.Sub(netMon.last) < 25*time.Second {
		netMon.Unlock()
		return
	}
	netMon.last = now
	netMon.Unlock()
	list := listAdapters()
	online := pcOnline()
	act := activeAdapter(list)
	key, label := "", "none"
	if act != nil {
		key, label = act.Kind+":"+act.Name, kindLabel(act.Kind)
		if act.SSID != "" {
			label += " (" + act.SSID + ")"
		}
	}
	netMon.Lock()
	defer netMon.Unlock()
	if netMon.known {
		if netMon.online && !online {
			netMon.offline = now
			netMonEvent("This PC lost its internet connection.")
			notify("error", "", "This PC is offline: sites hosted here are down until the connection is back.")
		} else if !netMon.online && online {
			d := now.Sub(netMon.offline).Round(time.Second)
			netMonEvent(fmt.Sprintf("Internet is back through %s (offline for %s).", label, d))
			notify("info", "", fmt.Sprintf("This PC is online again through %s.", label))
		} else if online && key != netMon.active && netMon.active != "" {
			netMonEvent("Internet now goes through " + label + ".")
			if act != nil && (act.Kind == "usb-tether" || act.Kind == "mobile") {
				notify("warn", "", "This PC switched to "+label+" (backup connection). Sites stay up through the Cloudflare tunnel; watch your data plan.")
			} else {
				notify("info", "", "This PC's internet now goes through "+label+".")
			}
		}
	}
	netMon.known, netMon.online, netMon.active = true, online, key
}

func netStatus() map[string]any {
	list := listAdapters()
	act := activeAdapter(list)
	netMon.Lock()
	defer netMon.Unlock()
	return map[string]any{"adapters": list, "active": act, "online": netMon.online || !netMon.known, "checked": netMon.known, "events": netMon.history}
}

// ---------- routes ----------

func thisPCState() map[string]any {
	c := snapshotConfig()
	cfg := ThisPCConfig{Port: 8080}
	if c.ThisPC != nil {
		cfg = *c.ThisPC
		if cfg.Port == 0 {
			cfg.Port = 8080
		}
	}
	localSrv.Lock()
	running, srvErr := localSrv.srv != nil, localSrv.err
	localSrv.Unlock()
	var sites []map[string]any
	for _, s := range c.Sites {
		for _, id := range siteServerIDs(s) {
			if id == thisPCID {
				sites = append(sites, map[string]any{"slug": s.Slug, "name": s.Name, "domain": s.Domain, "release": localLiveRelease(s.Slug),
					"url": fmt.Sprintf("http://%s.localhost:%d/", s.Slug, cfg.Port)})
			}
		}
	}
	p := findServer(&c, thisPCID)
	_, cfErr := os.Stat(cloudflaredPath())
	return map[string]any{
		"config": cfg, "running": running, "error": srvErr, "sites": sites,
		"tunnel":      p != nil && p.TunnelID != "",
		"tunnels":     localTunnels.status(),
		"cloudflared": cfErr == nil, "cloudflared_version": cloudflaredVersion,
		"cloudflare": c.CloudflareToken != "",
		"power":      powerStatus(), "keep_awake": keepAwakeOn(), "autostart": autostartOn(),
		"network":   netStatus(),
		"tradeoffs": homeTradeoffs, "windows": runtime.GOOS == "windows",
	}
}

func registerThisPCRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/thispc", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, thisPCState())
	})
	mux.HandleFunc("POST /api/thispc", func(w http.ResponseWriter, r *http.Request) {
		var req ThisPCConfig
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		if req.Port != 0 && (req.Port < 1024 || req.Port > 65535) {
			writeErr(w, errors.New("choose a port between 1024 and 65535 (8080 is a good default)"))
			return
		}
		err := withConfig(func(c *Config) error {
			x := req
			c.ThisPC = &x
			if x.Enabled {
				ensureThisPCProfile(c)
			}
			return nil
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		applyThisPC()
		writeJSON(w, 200, thisPCState())
	})
	mux.HandleFunc("POST /api/thispc/connect", func(w http.ResponseWriter, r *http.Request) {
		c := snapshotConfig()
		if !thisPCOn(&c) {
			writeErr(w, errors.New("turn on hosting on this PC first"))
			return
		}
		j := newJob("thispc-tunnel", func(j *Job) error {
			_, err := ensureServerTunnel(j, thisPCID)
			return err
		})
		writeJSON(w, 200, map[string]any{"job": j.ID})
	})
}
