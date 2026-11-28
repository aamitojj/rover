package main

// Self-update from GitHub Releases (v0.6).
//
// Rover asks the public GitHub API (no token, no account) for the newest
// release of the configured repository, shows its notes, downloads the
// installer (or rover.exe for portable copies), checks it against the
// SHA256SUMS.txt published in the same release (and GitHub's own asset
// digest when present, and an optional ed25519 signature), then installs it
// and restarts. Nothing here is required for Rover to work: when GitHub is
// unreachable the check simply reports "offline" and tries again later.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Version is the app version. It is a constant in spirit; it is a var only so
// release builds can set it with -ldflags "-X main.Version=x.y.z".
var Version = "0.9.0"

const defaultUpdateRepo = "aamitojj/rover"

// updateAPI is the GitHub REST API base (a var so tests can point it at a mock).
var updateAPI = "https://api.github.com"

// updatePublicKey is an optional base64 ed25519 public key. When a build sets
// it (-ldflags "-X main.updatePublicKey=..."), every release must also carry
// SHA256SUMS.txt.sig, a signature of SHA256SUMS.txt made with the private key.
// See docs/RELEASING.md.
var updatePublicKey = ""

// updateGOOS picks the download kind (a var so tests can act as Windows).
var updateGOOS = runtime.GOOS

// UpdateSettings live in Config.Update. All fields are optional.
type UpdateSettings struct {
	Repo        string `json:"repo,omitempty"`         // owner/name on GitHub
	AutoCheck   *bool  `json:"auto_check,omitempty"`   // default on: at startup and daily
	Install     string `json:"install,omitempty"`      // "ask" (default) or "auto"
	Channel     string `json:"channel,omitempty"`      // "stable" (default) or "beta"
	Skip        string `json:"skip,omitempty"`         // a version the user chose to skip
	RemindAfter string `json:"remind_after,omitempty"` // RFC3339: "Remind me later"
	LastCheck   string `json:"last_check,omitempty"`
}

func updateSettings(c *Config) UpdateSettings {
	u := UpdateSettings{}
	if c.Update != nil {
		u = *c.Update
	}
	if u.Repo == "" {
		u.Repo = defaultUpdateRepo
	}
	if u.Install != "auto" {
		u.Install = "ask"
	}
	if u.Channel != "beta" {
		u.Channel = "stable"
	}
	return u
}

func autoCheckOn(u UpdateSettings) bool { return u.AutoCheck == nil || *u.AutoCheck }

var repoRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)

// normalizeRepo accepts "owner/name", "https://github.com/owner/name(.git)".
func normalizeRepo(s string) (string, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "github.com/")
	s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	if !repoRe.MatchString(s) || strings.Contains(s, "..") {
		return "", fmt.Errorf("the repository must look like owner/name (got %q)", s)
	}
	return s, nil
}

// ---------- versions ----------

type semver struct {
	n   [3]int
	pre string
}

func parseSemver(v string) semver {
	v = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(v), "v"), "V")
	var out semver
	if i := strings.IndexAny(v, "+ "); i >= 0 {
		v = v[:i]
	}
	if i := strings.Index(v, "-"); i >= 0 {
		out.pre = v[i+1:]
		v = v[:i]
	}
	for i, p := range strings.SplitN(v, ".", 3) {
		n, _ := strconv.Atoi(p)
		out.n[i] = n
	}
	return out
}

func parseVer(v string) [3]int { return parseSemver(v).n }

// comparePre orders pre-release labels the semver way: numeric parts
// numerically, others alphabetically, shorter first when equal so far.
func comparePre(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, ea := strconv.Atoi(pa[i])
		nb, eb := strconv.Atoi(pb[i])
		switch {
		case ea == nil && eb == nil:
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
		case ea == nil:
			return -1
		case eb == nil:
			return 1
		default:
			if c := strings.Compare(pa[i], pb[i]); c != 0 {
				return c
			}
		}
	}
	switch {
	case len(pa) < len(pb):
		return -1
	case len(pa) > len(pb):
		return 1
	}
	return 0
}

// compareVersions returns -1, 0 or 1. Accepts "v1.2.3", "1.2", "1.2.3-beta.2".
// A pre-release sorts before its release (0.6.0-beta.1 < 0.6.0).
func compareVersions(a, b string) int {
	pa, pb := parseSemver(a), parseSemver(b)
	for i := 0; i < 3; i++ {
		if pa.n[i] != pb.n[i] {
			if pa.n[i] < pb.n[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case pa.pre == pb.pre:
		return 0
	case pa.pre == "":
		return 1
	case pb.pre == "":
		return -1
	}
	return comparePre(pa.pre, pb.pre)
}

// ---------- GitHub releases ----------

type ghAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"` // "sha256:<hex>" (GitHub computes it since June 2025)
}

type ghRelease struct {
	Tag        string    `json:"tag_name"`
	Name       string    `json:"name"`
	Body       string    `json:"body"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Published  string    `json:"published_at"`
	Page       string    `json:"html_url"`
	Assets     []ghAsset `json:"assets"`
}

func (r *ghRelease) version() string { return strings.TrimPrefix(strings.TrimPrefix(r.Tag, "v"), "V") }

func (r *ghRelease) asset(name string) *ghAsset {
	for i := range r.Assets {
		if strings.EqualFold(r.Assets[i].Name, name) {
			return &r.Assets[i]
		}
	}
	return nil
}

// UpdateInfo is what the UI shows. Offline/Error never block anything.
type UpdateInfo struct {
	Current    string `json:"current"`
	Latest     string `json:"latest,omitempty"`
	Newer      bool   `json:"newer"`
	Prerelease bool   `json:"prerelease,omitempty"`
	Title      string `json:"title,omitempty"`
	Notes      string `json:"notes,omitempty"`
	Published  string `json:"published,omitempty"`
	Page       string `json:"page,omitempty"`
	Asset      string `json:"asset,omitempty"`
	AssetSize  int64  `json:"asset_size,omitempty"`
	Kind       string `json:"kind,omitempty"` // installer | exe | "" (install by hand)
	Skipped    bool   `json:"skipped,omitempty"`
	Snoozed    bool   `json:"snoozed,omitempty"`
	Offline    bool   `json:"offline,omitempty"`
	NoReleases bool   `json:"no_releases,omitempty"`
	Error      string `json:"error,omitempty"`
	Checked    string `json:"checked,omitempty"`
	Repo       string `json:"repo"`
	Channel    string `json:"channel"`
}

var updState struct {
	sync.Mutex
	info     *UpdateInfo
	notified string
	auto     string // version an automatic install was started for
}

func cachedUpdateInfo() *UpdateInfo {
	updState.Lock()
	defer updState.Unlock()
	if updState.info == nil {
		return nil
	}
	cp := *updState.info
	return &cp
}

// allowedUpdateURL requires HTTPS, except for loopback (local testing).
func allowedUpdateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("update URLs must use https:// (got %q)", raw)
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// allowedDownloadURL: release files may only come from GitHub (or the mock
// API host in tests).
func allowedDownloadURL(raw string) error {
	if err := allowedUpdateURL(raw); err != nil {
		return err
	}
	u, _ := url.Parse(raw)
	h := strings.ToLower(u.Hostname())
	api, _ := url.Parse(updateAPI)
	switch {
	case h == "github.com", h == "api.github.com", strings.HasSuffix(h, ".githubusercontent.com"):
		return nil
	case api != nil && strings.EqualFold(api.Hostname(), h):
		return nil
	}
	return fmt.Errorf("refusing to download an update from %s (only GitHub is allowed)", h)
}

func updateClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return errors.New("too many redirects")
		}
		return allowedDownloadURL(req.URL.String())
	}}
}

var errNoReleases = errors.New("no releases published yet")

// isOffline reports network errors that mean "no internet / GitHub unreachable".
func isOffline(err error) bool {
	if err == nil {
		return false
	}
	var ne net.Error
	var de *net.DNSError
	var oe *net.OpError
	return errors.As(err, &de) || errors.As(err, &oe) || (errors.As(err, &ne) && ne.Timeout())
}

func ghGet(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(updateAPI, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "Rover/"+Version)
	resp, err := updateClient(20 * time.Second).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == 404:
		return errNoReleases
	case resp.StatusCode == 403 || resp.StatusCode == 429:
		return errors.New("GitHub asked us to slow down (rate limit). Rover tries again later")
	case resp.StatusCode != 200:
		return fmt.Errorf("GitHub answered HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(v)
}

// fetchRelease returns the newest release for the channel: stable uses
// /releases/latest (GitHub never returns drafts or pre-releases there); beta
// looks at the recent releases including pre-releases.
func fetchRelease(ctx context.Context, repo, channel string) (*ghRelease, error) {
	if channel == "beta" {
		var list []ghRelease
		if err := ghGet(ctx, "/repos/"+repo+"/releases?per_page=20", &list); err != nil {
			return nil, err
		}
		var best *ghRelease
		for i := range list {
			r := &list[i]
			if r.Draft || r.Tag == "" {
				continue
			}
			if best == nil || compareVersions(r.version(), best.version()) > 0 {
				best = r
			}
		}
		if best == nil {
			return nil, errNoReleases
		}
		return best, nil
	}
	var r ghRelease
	if err := ghGet(ctx, "/repos/"+repo+"/releases/latest", &r); err != nil {
		return nil, err
	}
	if r.Draft || r.Prerelease || r.Tag == "" {
		return nil, errNoReleases
	}
	return &r, nil
}

// installedCopy: the Inno Setup installer leaves unins000.exe next to the program.
func installedCopy() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(filepath.Dir(exe), "unins000.exe"))
	return err == nil
}

// pickAsset chooses what to download: the installer for installed copies,
// rover.exe for portable ones, whichever exists otherwise.
func pickAsset(r *ghRelease, goos string, installed bool) (*ghAsset, string) {
	if goos != "windows" {
		return nil, ""
	}
	// Rover's own names first; releases (or forks) that still publish the
	// Dilreet 0.6/0.7 names are accepted too.
	var setup *ghAsset
	ver := strings.ToLower(r.version())
	rank := func(n string) int {
		switch {
		case n == "rover-setup-"+ver+".exe":
			return 4
		case strings.HasPrefix(n, "rover-setup") && !strings.Contains(n, "offline") && strings.HasSuffix(n, ".exe"):
			return 3
		case n == "dilreet-setup-"+ver+".exe":
			return 2
		case strings.HasPrefix(n, "dilreet-setup") && strings.HasSuffix(n, ".exe"):
			return 1
		}
		return 0
	}
	best := 0
	for i := range r.Assets {
		if k := rank(strings.ToLower(r.Assets[i].Name)); k > best {
			best, setup = k, &r.Assets[i]
		}
	}
	exe := r.asset("rover.exe")
	if exe == nil {
		exe = r.asset("dilreet.exe")
	}
	switch {
	case installed && setup != nil:
		return setup, "installer"
	case exe != nil:
		return exe, "exe"
	case setup != nil:
		return setup, "installer"
	}
	return nil, ""
}

func infoFromRelease(r *ghRelease, u UpdateSettings, goos string, installed bool, now time.Time) *UpdateInfo {
	in := &UpdateInfo{Current: Version, Latest: r.version(), Prerelease: r.Prerelease, Title: r.Name, Notes: r.Body, Published: r.Published, Page: r.Page, Repo: u.Repo, Channel: u.Channel, Checked: now.UTC().Format(time.RFC3339)}
	in.Newer = compareVersions(in.Latest, Version) > 0
	if a, kind := pickAsset(r, goos, installed); a != nil {
		in.Asset, in.AssetSize, in.Kind = a.Name, a.Size, kind
	}
	in.Skipped = in.Newer && u.Skip != "" && compareVersions(u.Skip, in.Latest) == 0
	if t, err := time.Parse(time.RFC3339, u.RemindAfter); err == nil && now.Before(t) {
		in.Snoozed = in.Newer
	}
	if len(in.Notes) > 20000 {
		in.Notes = in.Notes[:20000] + "\n\n(…see the full notes on GitHub)"
	}
	return in
}

// checkForUpdate asks GitHub and caches the answer. It never returns an
// error: problems are reported in the info (Offline / Error).
func checkForUpdate(ctx context.Context) *UpdateInfo {
	c := snapshotConfig()
	u := updateSettings(&c)
	now := time.Now()
	in := &UpdateInfo{Current: Version, Repo: u.Repo, Channel: u.Channel, Checked: now.UTC().Format(time.RFC3339)}
	r, err := fetchRelease(ctx, u.Repo, u.Channel)
	switch {
	case errors.Is(err, errNoReleases):
		in.NoReleases = true
	case isOffline(err):
		in.Offline = true
		in.Error = "Could not reach GitHub. You may be offline; Rover checks again later."
	case err != nil:
		in.Error = err.Error()
	default:
		in = infoFromRelease(r, u, updateGOOS, installedCopy(), now)
	}
	updState.Lock()
	updState.info = in
	updState.Unlock()
	if in.Error == "" {
		_ = withConfig(func(c *Config) error {
			if c.Update == nil {
				c.Update = &UpdateSettings{}
			}
			c.Update.LastCheck = in.Checked
			return nil
		})
	}
	return in
}

// ---------- verification ----------

// parseSums reads "sha256sum" output: "<hex>  name" or "<hex> *name".
func parseSums(b []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(strings.TrimSpace(sc.Text()))
		if len(f) < 2 || len(f[0]) != 64 {
			continue
		}
		if _, err := hex.DecodeString(f[0]); err != nil {
			continue
		}
		name := strings.TrimPrefix(strings.Join(f[1:], " "), "*")
		name = name[strings.LastIndexAny(name, `/\`)+1:]
		out[strings.ToLower(name)] = strings.ToLower(f[0])
	}
	return out
}

func download(ctx context.Context, raw string, limit int64, w io.Writer) (int64, error) {
	if err := allowedDownloadURL(raw); err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Rover/"+Version)
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := updateClient(15 * time.Minute).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, limit+1))
	if err == nil && n > limit {
		err = errors.New("the download is larger than expected")
	}
	return n, err
}

// expectedSum downloads SHA256SUMS.txt (and checks its signature when a
// public key is built in) and returns the hash listed for the asset.
func expectedSum(ctx context.Context, r *ghRelease, a *ghAsset) (string, error) {
	sa := r.asset("SHA256SUMS.txt")
	if sa == nil {
		return "", errors.New("this release has no SHA256SUMS.txt, so it cannot be verified. The update was NOT installed")
	}
	var buf bytes.Buffer
	if _, err := download(ctx, sa.URL, 1<<20, &buf); err != nil {
		return "", fmt.Errorf("download SHA256SUMS.txt: %w", err)
	}
	if updatePublicKey != "" {
		if err := verifySumsSignature(ctx, r, buf.Bytes()); err != nil {
			return "", err
		}
	}
	want := parseSums(buf.Bytes())[strings.ToLower(a.Name)]
	if want == "" {
		return "", fmt.Errorf("SHA256SUMS.txt does not list %s. The update was NOT installed", a.Name)
	}
	if d := strings.ToLower(strings.TrimPrefix(a.Digest, "sha256:")); strings.HasPrefix(a.Digest, "sha256:") && d != want {
		return "", errors.New("GitHub's checksum and SHA256SUMS.txt disagree. The update was NOT installed")
	}
	return want, nil
}

func verifySumsSignature(ctx context.Context, r *ghRelease, sums []byte) error {
	pub, err := base64.StdEncoding.DecodeString(updatePublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("the built-in update key is invalid")
	}
	sa := r.asset("SHA256SUMS.txt.sig")
	if sa == nil {
		return errors.New("this release is not signed (SHA256SUMS.txt.sig is missing). The update was NOT installed")
	}
	var buf bytes.Buffer
	if _, err := download(ctx, sa.URL, 4096, &buf); err != nil {
		return fmt.Errorf("download the signature: %w", err)
	}
	sig := bytes.TrimSpace(buf.Bytes())
	if len(sig) != ed25519.SignatureSize {
		if d, err := base64.StdEncoding.DecodeString(string(sig)); err == nil {
			sig = d
		}
	}
	if len(sig) != ed25519.SignatureSize || !ed25519.Verify(ed25519.PublicKey(pub), sums, sig) {
		return errors.New("the release signature is not valid. The update was NOT installed")
	}
	return nil
}

// downloadVerified saves the asset to path and returns an error unless its
// sha256 matches SHA256SUMS.txt.
func downloadVerified(ctx context.Context, j *Job, r *ghRelease, a *ghAsset, path string) error {
	want, err := expectedSum(ctx, r, a)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("cannot save the download (%s): %w", path, err)
	}
	h := sha256.New()
	pw := &progressWriter{j: j, total: a.Size}
	n, err := download(ctx, a.URL, 500<<20, io.MultiWriter(f, h, pw))
	f.Close()
	if err != nil {
		os.Remove(path)
		return err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	j.Logf("Downloaded %.1f MB, sha256 %s", float64(n)/1e6, sum)
	if sum != want {
		os.Remove(path)
		return fmt.Errorf("checksum mismatch: SHA256SUMS.txt says %s. The update was NOT installed", want)
	}
	j.Logf("Checksum matches SHA256SUMS.txt.")
	return nil
}

type progressWriter struct {
	j          *Job
	total, n   int64
	lastReport int
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.n += int64(len(b))
	if p.j != nil && p.total > 0 {
		pct := int(p.n * 80 / p.total)
		if pct >= p.lastReport+5 {
			p.lastReport = pct
			p.j.SetProgress(5+pct, "Downloading")
		}
	}
	return len(b), nil
}

// ---------- install ----------

// installHook lets tests stop before replacing the program or restarting.
var installHook func(kind, path string) error

// updateJob downloads, verifies and installs version want (the one the user
// saw in the dialog). If GitHub now offers something else it stops.
func updateJob(want string) func(j *Job) error {
	return func(j *Job) error {
		ctx := context.Background()
		c := snapshotConfig()
		u := updateSettings(&c)
		j.SetProgress(2, "Checking GitHub")
		r, err := fetchRelease(ctx, u.Repo, u.Channel)
		if err != nil {
			if isOffline(err) {
				return errors.New("could not reach GitHub. Check your internet connection and try again")
			}
			return err
		}
		if want != "" && compareVersions(r.version(), want) != 0 {
			return fmt.Errorf("GitHub now offers %s instead of %s. Click \"Check for updates now\" and try again", r.version(), want)
		}
		if compareVersions(r.version(), Version) <= 0 {
			j.Logf("You already have the latest version (%s).", Version)
			return nil
		}
		a, kind := pickAsset(r, updateGOOS, installedCopy())
		if a == nil {
			return fmt.Errorf("release %s has no Windows download for this copy. Download it from %s", r.version(), r.Page)
		}
		j.Logf("Downloading %s %s (%.1f MB) from GitHub…", a.Name, r.version(), float64(a.Size)/1e6)
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		exe, _ = filepath.EvalSymlinks(exe)
		var path string
		if kind == "installer" {
			path = filepath.Join(os.TempDir(), "Rover-Setup-"+safeVer(r.version())+".exe")
		} else {
			path = exe + ".new"
		}
		if err := downloadVerified(ctx, j, r, a, path); err != nil {
			return err
		}
		j.SetProgress(90, "Installing")
		if installHook != nil {
			return installHook(kind, path)
		}
		if kind == "installer" {
			return runInstaller(j, path, r.version())
		}
		return swapExe(j, exe, path, r.version())
	}
}

func safeVer(v string) string {
	return strings.Map(func(r rune) rune {
		if r == '.' || r == '-' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return r
		}
		return '_'
	}, v)
}

// runInstaller starts the Inno Setup installer silently. Rover closes
// itself; the installer replaces the program and starts it again (see the
// [Run] entry with skipifnotsilent in installer/rover.iss).
func runInstaller(j *Job, path, ver string) error {
	cmd := exec.Command(path, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/CLOSEAPPLICATIONS", "/LOG="+filepath.Join(os.TempDir(), "Rover-update.log"))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start the installer: %w", err)
	}
	j.Logf("Installing %s in the background. Rover closes now and opens again by itself in about 20 seconds.", ver)
	go func() { time.Sleep(1500 * time.Millisecond); shutdown() }()
	return nil
}

// swapExe replaces a portable rover.exe (rename trick, works while running).
func swapExe(j *Job, exe, newPath, ver string) error {
	oldPath := exe + ".old"
	_ = os.Remove(oldPath)
	if err := os.Rename(exe, oldPath); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("cannot replace the program (%v). Move Rover to a folder you can write to, e.g. Documents", err)
	}
	if err := os.Rename(newPath, exe); err != nil {
		_ = os.Rename(oldPath, exe) // roll back
		return fmt.Errorf("install new program: %w", err)
	}
	if appMode == modeNative {
		j.Logf("Installed %s. Rover restarts in a moment (the window reopens by itself)…", ver)
	} else {
		j.Logf("Installed %s. Restarting Rover (a new browser tab will open)…", ver)
	}
	go func() {
		time.Sleep(1500 * time.Millisecond)
		args := []string{"-after-update"}
		for _, a := range os.Args[1:] {
			if a != "-after-update" && a != "--after-update" {
				args = append(args, a)
			}
		}
		cmd := exec.Command(exe, args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			fmt.Println("restart failed:", err, "- please start Rover again manually")
		}
		shutdown()
	}()
	return nil
}

// cleanupOldBinary removes the .old file left by a previous self-update.
func cleanupOldBinary() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	exe, _ = filepath.EvalSymlinks(exe)
	go func() {
		for i := 0; i < 10; i++ {
			if err := os.Remove(exe + ".old"); err == nil || os.IsNotExist(err) {
				return
			}
			time.Sleep(time.Second) // the old process may still be exiting (Windows locks)
		}
	}()
}

// ---------- automatic checks ----------

// startUpdater checks shortly after startup and then once a day (when
// "Automatically check" is on), notifies once per version and installs by
// itself when the user chose "Automatically download and install".
func startUpdater(ctx context.Context) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(45 * time.Second):
		}
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			updaterTick(ctx, time.Now())
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

var startedAt = time.Now()

func updaterTick(ctx context.Context, now time.Time) {
	defer func() { _ = recover() }()
	c := snapshotConfig()
	u := updateSettings(&c)
	if !autoCheckOn(u) {
		return
	}
	last, _ := time.Parse(time.RFC3339, u.LastCheck)
	in := cachedUpdateInfo()
	due := now.Sub(last) > 23*time.Hour || (in == nil && now.Sub(startedAt) < 2*time.Hour)
	if in != nil && in.Offline && now.Sub(last) > 3*time.Hour {
		due = true
	}
	if !due {
		return
	}
	in = checkForUpdate(ctx)
	if !in.Newer || in.Skipped || in.Snoozed {
		return
	}
	updState.Lock()
	first := updState.notified != in.Latest
	updState.notified = in.Latest
	autoGo := u.Install == "auto" && in.Kind != "" && updState.auto != in.Latest
	updState.Unlock()
	if autoGo && !anyJobRunning() {
		updState.Lock()
		updState.auto = in.Latest
		updState.Unlock()
		notify("info", "", fmt.Sprintf("Installing Rover %s automatically (you chose automatic updates). Rover restarts by itself.", in.Latest))
		newJob("self-update", updateJob(in.Latest))
		return
	}
	if first {
		notify("info", "", fmt.Sprintf("Rover %s is available. Open Settings > Updates to see what is new and install it.", in.Latest))
	}
}

// ---------- API ----------

func updateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/update/status", func(w http.ResponseWriter, r *http.Request) {
		c := snapshotConfig()
		u := updateSettings(&c)
		writeJSON(w, 200, map[string]any{"current": Version, "info": cachedUpdateInfo(), "settings": u, "auto_check": autoCheckOn(u), "installed": installedCopy(), "os": updateGOOS, "signed": updatePublicKey != ""})
	})
	mux.HandleFunc("GET /api/update/check", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		writeJSON(w, 200, checkForUpdate(ctx))
	})
	mux.HandleFunc("POST /api/update/settings", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Repo      *string `json:"repo"`
			AutoCheck *bool   `json:"auto_check"`
			Install   *string `json:"install"`
			Channel   *string `json:"channel"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		repo := ""
		if req.Repo != nil && strings.TrimSpace(*req.Repo) != "" {
			var err error
			if repo, err = normalizeRepo(*req.Repo); err != nil {
				writeErr(w, err)
				return
			}
		}
		_ = withConfig(func(c *Config) error {
			if c.Update == nil {
				c.Update = &UpdateSettings{}
			}
			if req.Repo != nil {
				if repo == defaultUpdateRepo {
					repo = ""
				}
				if c.Update.Repo != repo {
					c.Update.LastCheck = ""
				}
				c.Update.Repo = repo
			}
			if req.AutoCheck != nil {
				v := *req.AutoCheck
				c.Update.AutoCheck = &v
			}
			if req.Install != nil {
				c.Update.Install = map[bool]string{true: "auto", false: "ask"}[*req.Install == "auto"]
			}
			if req.Channel != nil {
				c.Update.Channel = map[bool]string{true: "beta", false: "stable"}[*req.Channel == "beta"]
				c.Update.LastCheck = ""
			}
			return nil
		})
		c := snapshotConfig()
		writeJSON(w, 200, map[string]any{"ok": true, "settings": updateSettings(&c)})
	})
	mux.HandleFunc("POST /api/update/skip", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Version string `json:"version"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		_ = withConfig(func(c *Config) error {
			if c.Update == nil {
				c.Update = &UpdateSettings{}
			}
			c.Update.Skip = strings.TrimSpace(req.Version)
			return nil
		})
		updState.Lock()
		if updState.info != nil && req.Version != "" {
			updState.info.Skipped = compareVersions(req.Version, updState.info.Latest) == 0
		}
		updState.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/update/later", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Hours int `json:"hours"`
		}
		_ = readJSON(r, &req)
		if req.Hours <= 0 || req.Hours > 24*30 {
			req.Hours = 24
		}
		until := time.Now().Add(time.Duration(req.Hours) * time.Hour).UTC().Format(time.RFC3339)
		_ = withConfig(func(c *Config) error {
			if c.Update == nil {
				c.Update = &UpdateSettings{}
			}
			c.Update.RemindAfter = until
			return nil
		})
		updState.Lock()
		if updState.info != nil {
			updState.info.Snoozed = updState.info.Newer
		}
		updState.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true, "until": until})
	})
	mux.HandleFunc("POST /api/update/apply", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Version string `json:"version"`
		}
		_ = readJSON(r, &req)
		j := newJob("self-update", updateJob(strings.TrimSpace(req.Version)))
		writeJSON(w, 200, map[string]any{"job": j.ID})
	})
}

// latestCoolifyVersion reads the official versions file used by Coolify's installer.
func latestCoolifyVersion() (string, error) {
	cl := &http.Client{Timeout: 15 * time.Second}
	resp, err := cl.Get("https://cdn.coollabs.io/coolify/versions.json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var v struct {
		Coolify struct {
			V4 struct {
				Version string `json:"version"`
			} `json:"v4"`
		} `json:"coolify"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&v); err != nil {
		return "", err
	}
	return v.Coolify.V4.Version, nil
}

// The official installer also upgrades an existing installation to the latest
// version (it downloads and runs /data/coolify/source/upgrade.sh).
const coolifyInstallCmd = "curl -fsSL https://cdn.coollabs.io/coolify/install.sh | bash"

func installCoolifyJob(j *Job, upgrade bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	j.Logf("Checking the server...")
	pre := `echo "OS: $(. /etc/os-release 2>/dev/null; echo $PRETTY_NAME)"; echo "Arch: $(uname -m)"; echo "CPU cores: $(nproc)"; echo "RAM: $(free -m | awk '/Mem:/{print $2}') MB"; echo "Free disk on /: $(df -BG / | awk 'NR==2{print $4}')"`
	if err := runRemote(ctx, pre, j); err != nil {
		return err
	}
	if upgrade {
		j.Logf("Updating Coolify to the latest version by re-running the official installer...")
		j.Logf("Running deployments may fail during the update; the dashboard is down for about a minute.")
	} else {
		j.Logf("Installing Coolify with the official installer. This usually takes 2-10 minutes.")
	}
	j.Logf("$ %s", coolifyInstallCmd)
	j.SetProgress(2, "Starting the installer")
	j.mu.Lock()
	j.onLine = installLineHook
	j.mu.Unlock()
	if err := runRemote(ctx, coolifyInstallCmd, j); err != nil {
		return err
	}
	c := snapshotConfig()
	if !upgrade {
		j.Logf("")
		j.Logf("Coolify is installed. IMPORTANT: open http://%s:8000 NOW and create the admin account (the first visitor becomes admin).", c.ServerIP)
		j.Logf("Back up /data/coolify/source/.env (it contains the encryption key).")
	}
	return nil
}

func osName() string { return runtime.GOOS + "/" + runtime.GOARCH }
