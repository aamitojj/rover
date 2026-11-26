package main

// v0.6 project hub: per-project settings (publish lock, build settings,
// password protection, primary domain, alerts), drafts, the redirects and
// headers editor, "redeploy last folder", recent folders, Explorer
// "Deploy with Rover" hand-off and incremental (changed files only) uploads.

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// SiteAccess is nginx basic-auth password protection for one project.
type SiteAccess struct {
	User string `json:"user"`
	Hash string `json:"hash"` // apr1 hash, protected with DPAPI on Windows
}

func boolOn(p *bool) bool { return p == nil || *p }

// ---------- apr1 (Apache MD5) for nginx auth_basic_user_file ----------

const apr1Alphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func apr1(password, salt string) string {
	if len(salt) > 8 {
		salt = salt[:8]
	}
	magic := "$apr1$"
	pw := []byte(password)
	alt := md5.Sum([]byte(password + salt + password))
	ctx := md5.New()
	ctx.Write([]byte(password + magic + salt))
	for i := len(pw); i > 0; i -= 16 {
		n := i
		if n > 16 {
			n = 16
		}
		ctx.Write(alt[:n])
	}
	for i := len(pw); i > 0; i >>= 1 {
		if i&1 == 1 {
			ctx.Write([]byte{0})
		} else {
			ctx.Write(pw[:1])
		}
	}
	final := ctx.Sum(nil)
	for i := 0; i < 1000; i++ {
		c := md5.New()
		if i&1 == 1 {
			c.Write(pw)
		} else {
			c.Write(final)
		}
		if i%3 != 0 {
			c.Write([]byte(salt))
		}
		if i%7 != 0 {
			c.Write(pw)
		}
		if i&1 == 1 {
			c.Write(final)
		} else {
			c.Write(pw)
		}
		final = c.Sum(nil)
	}
	var out strings.Builder
	enc := func(a, b, c byte, n int) {
		v := uint(a)<<16 | uint(b)<<8 | uint(c)
		for ; n > 0; n-- {
			out.WriteByte(apr1Alphabet[v&0x3f])
			v >>= 6
		}
	}
	enc(final[0], final[6], final[12], 4)
	enc(final[1], final[7], final[13], 4)
	enc(final[2], final[8], final[14], 4)
	enc(final[3], final[9], final[15], 4)
	enc(final[4], final[10], final[5], 4)
	enc(0, 0, final[11], 2)
	return magic + salt + "$" + out.String()
}

func newSalt() string {
	b := randHex(8)
	var s strings.Builder
	for i := 0; i < 8; i++ {
		s.WriteByte(apr1Alphabet[int(b[i])%len(apr1Alphabet)])
	}
	return s.String()
}

var accessUserRe = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,64}$`)

// htpasswdFor returns the .htpasswd content for a protected site ("" = off).
func htpasswdFor(s *Site) string {
	if s == nil || s.Access == nil || s.Access.User == "" || s.Access.Hash == "" {
		return ""
	}
	h, err := unprotectString(s.Access.Hash)
	if err != nil || !strings.HasPrefix(h, "$apr1$") {
		return ""
	}
	return s.Access.User + ":" + h + "\n"
}

// siteConfExtras adds project settings to the generated nginx config:
// password protection and the primary-domain redirect (www <-> bare domain).
func siteConfExtras(conf string, s *Site) string {
	if s == nil {
		return conf
	}
	var b strings.Builder
	if htpasswdFor(s) != "" {
		b.WriteString("    # Rover: password protection (Project > Configuration > Access)\n    auth_basic \"Private site\";\n    auth_basic_user_file /etc/nginx/conf.d/.htpasswd;\n")
	}
	if s.Domain != "" && s.IncludeWWW && (s.Primary == "apex" || s.Primary == "www") {
		scheme := "http"
		if s.HTTPSActive {
			scheme = "https"
		}
		from, to := "www."+s.Domain, s.Domain
		if s.Primary == "www" {
			from, to = s.Domain, "www."+s.Domain
		}
		fmt.Fprintf(&b, "    # Rover: primary domain is %s\n    if ($host = %s) { return 301 %s://%s$request_uri; }\n", to, from, scheme, to)
	}
	if b.Len() == 0 {
		return conf
	}
	const anchor = "    index index.html;\n"
	i := strings.Index(conf, anchor)
	if i < 0 {
		return conf
	}
	i += len(anchor)
	return conf[:i] + b.String() + conf[i:]
}

// htpasswdScript writes or removes $BASE/nginx/.htpasswd on the server.
func htpasswdScript(s *Site) string {
	hp := htpasswdFor(s)
	if hp == "" {
		return `rm -f "$BASE/nginx/.htpasswd"`
	}
	d := "HPPW_" + randHex(6)
	return fmt.Sprintf("cat > \"$BASE/nginx/.htpasswd\" <<'%s'\n%s%s\nchmod 644 \"$BASE/nginx/.htpasswd\"", d, hp, d)
}

// ---------- drafts and publish folder ----------

// saveDraft keeps the deploy on this PC (Deploys list, "Draft") without
// touching the server. Publish it later from the Deploys tab.
func saveDraft(j *Job, slug string, ws *Workspace) error {
	if ws.Files == nil || !ws.Files.IsFile("index.html") {
		return errors.New("no index.html found at the top level of the publish folder")
	}
	rec := archiveDeploy(j, slug, ws)
	if rec == nil {
		return errors.New("could not save the draft on this PC (see the log)")
	}
	j.Logf("Saved as a draft (%d files). The live site was not changed. Open Deploys > Preview or Publish.", rec.Files)
	j.SetResult(map[string]any{"draft": rec.ID})
	return nil
}

// applyPublishOverride serves the folder chosen in Build & deploy settings.
func applyPublishOverride(j *Job, s *Site, ws *Workspace) error {
	if s == nil || s.PublishDir == "" || ws.PublishRel == s.PublishDir || ws.restoreOf != "" {
		return nil
	}
	rel, err := cleanRel(s.PublishDir)
	if err != nil || !isFileOS(filepath.Join(ws.Root, filepath.FromSlash(rel), "index.html")) {
		j.Logf("Note: the publish folder %q from the project settings is not in this upload; using %s instead.", s.PublishDir, firstNonEmpty(ws.PublishRel, "the top level"))
		return nil
	}
	ws.PublishRel, ws.PublishNote = rel, "from the project settings (publish folder)"
	ws.Publish = filepath.Join(ws.Root, filepath.FromSlash(rel))
	ws.Rules = loadSiteRules(ws.Root, ws.Publish)
	ws.Bytes = 0
	return ws.index()
}

// ---------- incremental uploads ----------

type fileManifest struct {
	Release string            `json:"release"`
	Files   map[string]string `json:"files"` // path -> sha256
}

var manifestMu sync.Mutex

func manifestPath(slug string) string {
	d, err := configDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "manifests", slug+".json")
}

func loadManifest(slug string) *fileManifest {
	p := manifestPath(slug)
	if p == "" {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	m := &fileManifest{}
	if json.Unmarshal(b, m) != nil || m.Files == nil {
		return nil
	}
	return m
}

func saveManifest(slug string, m *fileManifest) {
	p := manifestPath(slug)
	if p == "" || m == nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	b, _ := json.Marshal(m)
	_ = os.WriteFile(p, b, 0o600)
}

func hashEntries(entries []srcEntry) (map[string]string, error) {
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		rc, err := e.Open()
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		_, err = io.Copy(h, rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		out[e.Path] = hex.EncodeToString(h.Sum(nil))
	}
	return out, nil
}

// incPlan is a changed-files-only upload on top of the live release.
type incPlan struct {
	Prev    string
	Deleted []string
}

// diffManifest returns the changed/new paths and the deleted paths.
func diffManifest(old, cur map[string]string) (changed, deleted []string) {
	for p, h := range cur {
		if old[p] != h {
			changed = append(changed, p)
		}
	}
	for p := range old {
		if _, ok := cur[p]; !ok {
			deleted = append(deleted, p)
		}
	}
	sort.Strings(changed)
	sort.Strings(deleted)
	return
}

var releaseNameRe = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}$`)

// incPrelude is the server script part that seeds the new release with the
// live one and removes deleted files. Exit 42 = "send everything instead".
func incPrelude(p *incPlan) string {
	if p == nil {
		return ""
	}
	d := "HPDEL_" + randHex(6)
	var b strings.Builder
	fmt.Fprintf(&b, "PREV=\"$BASE/releases/%s\"\n", p.Prev)
	fmt.Fprintf(&b, "if [ ! -d \"$PREV\" ] || [ \"$(readlink \"$BASE/html\")\" != \"releases/%s\" ]; then rmdir \"$REL\"; echo \"HP_NEED_FULL: the live release changed; sending all files\"; exit 42; fi\n", p.Prev)
	b.WriteString("cp -a \"$PREV/.\" \"$REL/\"\n")
	if len(p.Deleted) > 0 {
		fmt.Fprintf(&b, "while IFS= read -r f; do [ -n \"$f\" ] && rm -f -- \"$REL/$f\"; done <<'%s'\n", d)
		for _, f := range p.Deleted {
			b.WriteString(f + "\n")
		}
		b.WriteString(d + "\n")
	}
	return b.String()
}

func safeManifestPath(p string) bool {
	if p == "" || strings.ContainsAny(p, "\n\r\x00") {
		return false
	}
	c, err := cleanRel(p)
	return err == nil && c == p
}

// deployMaybeIncremental uploads only changed files when the server still
// runs the release recorded in the local manifest; otherwise everything.
func deployMaybeIncremental(j *Job, slug string, entries []srcEntry, tarball string, n int, size int64, conf string) error {
	c := snapshotConfig()
	site := findSite(&c, slug)
	if site == nil {
		return errors.New("unknown site")
	}
	entries = stripCommonRoot(entries)
	cur, herr := hashEntries(entries)
	full := func() error {
		err := deploySiteJobX(j, slug, tarball, n, size, conf, nil)
		if err == nil && herr == nil {
			if s, e := siteOr404(slug); e == nil && s.Release != "" {
				manifestMu.Lock()
				saveManifest(slug, &fileManifest{Release: s.Release, Files: cur})
				manifestMu.Unlock()
			}
		}
		return err
	}
	if herr != nil || c.NoIncremental || site.Release == "" || !releaseNameRe.MatchString(site.Release) {
		return full()
	}
	manifestMu.Lock()
	old := loadManifest(slug)
	manifestMu.Unlock()
	if old == nil || old.Release != site.Release {
		return full()
	}
	changed, deleted := diffManifest(old.Files, cur)
	for _, p := range deleted {
		if !safeManifestPath(p) {
			return full()
		}
	}
	byPath := map[string]srcEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	var part []srcEntry
	var partSize int64
	for _, p := range changed {
		part = append(part, byPath[p])
		partSize += byPath[p].Size
	}
	if size > 0 && partSize*10 > size*7 { // most files changed: a full upload is as fast
		return full()
	}
	small, sn, ssize, err := writeTarball(part, partSize)
	if err != nil {
		return full()
	}
	defer os.Remove(small)
	j.Logf("Fast deploy: %d changed and %d removed files (of %d); the other files are copied on the server from the live version.", len(changed), len(deleted), len(cur))
	err = deploySiteJobX(j, slug, small, sn, ssize, conf, &incPlan{Prev: site.Release, Deleted: deleted})
	if err != nil && strings.Contains(err.Error(), "status 42") {
		j.Logf("The server's live version is not the one Rover expected; sending all files instead.")
		return full()
	}
	if err == nil {
		if s, e := siteOr404(slug); e == nil && s.Release != "" {
			manifestMu.Lock()
			saveManifest(slug, &fileManifest{Release: s.Release, Files: cur})
			manifestMu.Unlock()
		}
	}
	return err
}

// ---------- recent folders, quick deploy (Explorer right-click) ----------

func rememberFolder(p string) {
	p = strings.TrimSpace(p)
	if p == "" {
		return
	}
	_ = withConfig(func(c *Config) error {
		out := []string{p}
		for _, x := range c.RecentFolders {
			if !strings.EqualFold(filepath.Clean(x), filepath.Clean(p)) {
				out = append(out, x)
			}
		}
		if len(out) > 8 {
			out = out[:8]
		}
		c.RecentFolders = out
		return nil
	})
}

var quick struct {
	sync.Mutex
	path string
	at   time.Time
}

// setQuickDeploy is called for "rover.exe -deploy <folder>" (the Explorer
// menu). The UI asks which project to deploy to and confirms first.
func setQuickDeploy(p string) {
	p = strings.Trim(strings.TrimSpace(p), `"`)
	if p == "" {
		return
	}
	quick.Lock()
	quick.path, quick.at = p, time.Now()
	quick.Unlock()
}

func takeQuickDeploy() string {
	quick.Lock()
	defer quick.Unlock()
	if quick.path == "" || time.Since(quick.at) > 10*time.Minute {
		quick.path = ""
		return ""
	}
	return quick.path
}

// ---------- helpers for the rules editor ----------

// currentPublishDir is where the live files' _redirects/_headers belong.
func currentPublishDir(dir string, s *Site) (string, string) {
	cur := filepath.Join(dir, "current")
	try := []string{}
	if s.PublishDir != "" {
		try = append(try, s.PublishDir)
	}
	if ix, err := readIndex(dir); err == nil {
		for _, d := range ix.Deploys {
			if d.Status == "published" {
				try = append(try, d.Publish)
				break
			}
		}
	}
	try = append(try, "")
	try = append(try, outputDirCandidates...)
	for _, rel := range try {
		r, err := cleanRel(rel)
		if err != nil {
			continue
		}
		p := filepath.Join(cur, filepath.FromSlash(r))
		if isFileOS(filepath.Join(p, "index.html")) {
			return p, r
		}
	}
	return cur, ""
}

func readSmall(p string) string {
	b, err := os.ReadFile(p)
	if err != nil || len(b) > 1<<20 {
		return ""
	}
	return string(b)
}

func validateRules(redirects, headers string) []RuleIssue {
	s := &SiteRules{}
	if strings.TrimSpace(redirects) != "" {
		s.parseRedirectsFile(redirects, "_redirects")
	}
	if strings.TrimSpace(headers) != "" {
		s.parseHeadersFile(headers, "_headers")
	}
	if s.Issues == nil {
		return []RuleIssue{}
	}
	return s.Issues
}

func writeOrRemove(p, text string) error {
	if strings.TrimSpace(text) == "" {
		err := os.Remove(p)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return os.WriteFile(p, []byte(text), 0o644)
}

// deployLocal deploys a folder or zip on this PC to a project (job body).
func deployLocal(j *Job, slug, p string, draft bool) error {
	var entries []srcEntry
	var cleanup func()
	var err error
	st, e := os.Stat(p)
	switch {
	case e != nil:
		return fmt.Errorf("folder not found: %s", p)
	case st.IsDir():
		entries, err = dirEntries(p)
	case strings.EqualFold(filepath.Ext(p), ".zip"):
		entries, cleanup, err = zipEntries(p)
	default:
		return errors.New("choose a folder or a .zip file")
	}
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}
	ws, err := newWorkspace(entries, p)
	if err != nil {
		return err
	}
	defer ws.Close()
	ws.LocalPath, ws.draft = p, draft
	rep := runChecks(ws)
	j.Logf("Checks: %d passed, %d warnings, %d failed.", rep.Counts["pass"], rep.Counts["warn"], rep.Counts["fail"])
	return deployWorkspace(j, slug, ws)
}

// ---------- API ----------

type siteConfigView struct {
	Slug        string   `json:"slug"`
	Locked      bool     `json:"locked"`
	BuildCmd    string   `json:"build_cmd"`
	PublishDir  string   `json:"publish_dir"`
	Framework   string   `json:"framework"`
	Primary     string   `json:"primary"`
	AlertDown   bool     `json:"alert_down"`
	AlertFail   bool     `json:"alert_fail"`
	NoBackup    bool     `json:"no_backup"`
	AccessOn    bool     `json:"access_on"`
	AccessUser  string   `json:"access_user"`
	LastSource  string   `json:"last_source"`
	Recent      []string `json:"recent"`
	Incremental bool     `json:"incremental"`
	Detected    string   `json:"detected_publish"`
}

func viewSiteConfig(c *Config, s *Site) siteConfigView {
	v := siteConfigView{Slug: s.Slug, Locked: s.Locked, BuildCmd: s.BuildCmd, PublishDir: s.PublishDir, Framework: s.Framework, Primary: s.Primary, AlertDown: boolOn(s.AlertDown), AlertFail: boolOn(s.AlertFail), NoBackup: s.NoBackup, LastSource: s.LastSource, Recent: c.RecentFolders, Incremental: !c.NoIncremental}
	if s.Access != nil && s.Access.Hash != "" {
		v.AccessOn, v.AccessUser = true, s.Access.User
	}
	if v.Recent == nil {
		v.Recent = []string{}
	}
	if dir, _, err := projectDir(s.Slug); err == nil {
		_, v.Detected = currentPublishDir(dir, s)
	}
	return v
}

var buildCmdRe = regexp.MustCompile(`^npm run [A-Za-z0-9:._-]{1,40}$`)

func project06Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sites/{slug}/config", func(w http.ResponseWriter, r *http.Request) {
		c := snapshotConfig()
		s := findSite(&c, r.PathValue("slug"))
		if s == nil {
			writeErr(w, errors.New("project not found"))
			return
		}
		writeJSON(w, 200, viewSiteConfig(&c, s))
	})
	mux.HandleFunc("POST /api/sites/{slug}/config", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Locked      *bool   `json:"locked"`
			BuildCmd    *string `json:"build_cmd"`
			PublishDir  *string `json:"publish_dir"`
			Primary     *string `json:"primary"`
			AlertDown   *bool   `json:"alert_down"`
			AlertFail   *bool   `json:"alert_fail"`
			NoBackup    *bool   `json:"no_backup"`
			Incremental *bool   `json:"incremental"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		if req.BuildCmd != nil {
			*req.BuildCmd = strings.Join(strings.Fields(*req.BuildCmd), " ")
			if *req.BuildCmd != "" && !buildCmdRe.MatchString(*req.BuildCmd) {
				writeErr(w, errors.New(`the build command must look like "npm run build" (other commands are not run, for safety)`))
				return
			}
		}
		if req.PublishDir != nil {
			p := strings.Trim(strings.ReplaceAll(strings.TrimSpace(*req.PublishDir), `\`, "/"), "/")
			p = strings.TrimPrefix(p, "./")
			if p != "" {
				c, err := cleanRel(p)
				if err != nil || c != path.Clean(p) {
					writeErr(w, errors.New("the publish folder must be a folder inside your project, e.g. dist"))
					return
				}
				p = c
			}
			*req.PublishDir = p
		}
		if req.Primary != nil && *req.Primary != "" && *req.Primary != "apex" && *req.Primary != "www" {
			writeErr(w, errors.New("primary must be apex or www"))
			return
		}
		slug := r.PathValue("slug")
		err := withConfig(func(c *Config) error {
			s := findSite(c, slug)
			if s == nil {
				return errors.New("project not found")
			}
			if req.Locked != nil {
				s.Locked = *req.Locked
			}
			if req.BuildCmd != nil {
				s.BuildCmd = *req.BuildCmd
			}
			if req.PublishDir != nil {
				s.PublishDir = *req.PublishDir
			}
			if req.Primary != nil {
				s.Primary = *req.Primary
			}
			if req.AlertDown != nil {
				v := *req.AlertDown
				s.AlertDown = &v
			}
			if req.AlertFail != nil {
				v := *req.AlertFail
				s.AlertFail = &v
			}
			if req.NoBackup != nil {
				s.NoBackup = *req.NoBackup
			}
			if req.Incremental != nil {
				c.NoIncremental = !*req.Incremental
			}
			return nil
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		c := snapshotConfig()
		writeJSON(w, 200, viewSiteConfig(&c, findSite(&c, slug)))
	})
	mux.HandleFunc("POST /api/sites/{slug}/access", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Enabled  bool   `json:"enabled"`
			User     string `json:"user"`
			Password string `json:"password"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		var acc *SiteAccess
		if req.Enabled {
			req.User = strings.TrimSpace(req.User)
			if !accessUserRe.MatchString(req.User) {
				writeErr(w, errors.New("the user name may use letters, numbers and . _ @ - (up to 64)"))
				return
			}
			if len(req.Password) < 8 || len(req.Password) > 128 || strings.ContainsAny(req.Password, "\n\r") {
				writeErr(w, errors.New("use a password of at least 8 characters"))
				return
			}
			h, err := protectString(apr1(req.Password, newSalt()))
			if err != nil {
				writeErr(w, err)
				return
			}
			acc = &SiteAccess{User: req.User, Hash: h}
		}
		slug := r.PathValue("slug")
		err := withConfig(func(c *Config) error {
			s := findSite(c, slug)
			if s == nil {
				return errors.New("project not found")
			}
			s.Access = acc
			return nil
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "access_on": acc != nil})
	})
	mux.HandleFunc("GET /api/projects/{slug}/rules", func(w http.ResponseWriter, r *http.Request) {
		dir, s, err := projectDir(r.PathValue("slug"))
		if err != nil {
			writeErr(w, err)
			return
		}
		pd, rel := currentPublishDir(dir, s)
		red, hdr := readSmall(filepath.Join(pd, "_redirects")), readSmall(filepath.Join(pd, "_headers"))
		toml := readSmall(filepath.Join(dir, "current", "netlify.toml"))
		writeJSON(w, 200, map[string]any{"redirects": red, "headers": hdr, "publish": rel, "has_toml": toml != "", "issues": validateRules(red, hdr), "has_files": isFileOS(filepath.Join(pd, "index.html"))})
	})
	mux.HandleFunc("POST /api/projects/{slug}/rules", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Redirects string `json:"redirects"`
			Headers   string `json:"headers"`
			CheckOnly bool   `json:"check_only"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		issues := validateRules(req.Redirects, req.Headers)
		fails := 0
		for _, i := range issues {
			if i.Level == "fail" {
				fails++
			}
		}
		if req.CheckOnly || fails > 0 {
			writeJSON(w, 200, map[string]any{"saved": false, "issues": issues, "fails": fails})
			return
		}
		dir, s, err := projectDir(r.PathValue("slug"))
		if err != nil {
			writeErr(w, err)
			return
		}
		pd, _ := currentPublishDir(dir, s)
		if !isFileOS(filepath.Join(pd, "index.html")) {
			writeErr(w, errors.New("this project has no files on this PC yet. Deploy once, then edit its rules here"))
			return
		}
		if err := writeOrRemove(filepath.Join(pd, "_redirects"), req.Redirects); err != nil {
			writeErr(w, err)
			return
		}
		if err := writeOrRemove(filepath.Join(pd, "_headers"), req.Headers); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"saved": true, "issues": issues, "fails": 0})
	})
	// apply: publish the project's current files again (after rules,
	// password or primary-domain changes). Fast with incremental uploads.
	mux.HandleFunc("POST /api/sites/{slug}/apply", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		dir, _, err := projectDir(slug)
		if err != nil {
			writeErr(w, err)
			return
		}
		if !sshS.connected() {
			writeErr(w, errNotConnected)
			return
		}
		cur := filepath.Join(dir, "current")
		if !isDirOS(cur) {
			writeErr(w, errors.New("this project has no files on this PC yet"))
			return
		}
		j := newJob("apply-settings", func(j *Job) error {
			j.Logf("Publishing the current files again with the new settings…")
			return deployLocal(j, slug, cur, false)
		})
		writeJSON(w, 200, map[string]any{"job": j.ID})
	})
	mux.HandleFunc("POST /api/sites/{slug}/redeploy-last", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path  string `json:"path"`
			Draft bool   `json:"draft"`
		}
		_ = readJSON(r, &req)
		slug := r.PathValue("slug")
		s, err := siteOr404(slug)
		if err != nil {
			writeErr(w, err)
			return
		}
		p := strings.Trim(strings.TrimSpace(req.Path), `"`)
		if p == "" {
			p = firstNonEmpty(s.WatchPath, s.LastSource)
		}
		if p == "" || !filepath.IsAbs(p) {
			writeErr(w, errors.New("Rover does not know a folder on this PC for this project yet. Drop the folder once, or enter its path"))
			return
		}
		if !req.Draft && !sshS.connected() {
			writeErr(w, errNotConnected)
			return
		}
		rememberFolder(p)
		j := newJob("deploy", func(j *Job) error {
			j.Logf("Deploying %s", p)
			err := deployLocal(j, slug, p, req.Draft)
			if err == nil {
				_ = withConfig(func(c *Config) error {
					if st := findSite(c, slug); st != nil {
						st.LastSource = p
					}
					return nil
				})
			}
			return err
		})
		writeJSON(w, 200, map[string]any{"job": j.ID, "path": p})
	})
	mux.HandleFunc("POST /api/projects/{slug}/deploys/{id}/preview", func(w http.ResponseWriter, r *http.Request) {
		dir, _, _, rec, ok := deployRecOr404(w, r)
		if !ok {
			return
		}
		entries, cleanup, err := zipEntries(filepath.Join(dir, "deploys", rec.File))
		if err != nil {
			writeErr(w, err)
			return
		}
		defer cleanup()
		ws, err := newWorkspace(entries, "deploy "+rec.ID)
		if err != nil {
			writeErr(w, err)
			return
		}
		t, err := startTest(ws, r.Host)
		if err != nil {
			ws.Close()
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, testState(t))
	})
	mux.HandleFunc("GET /api/quick", func(w http.ResponseWriter, r *http.Request) {
		c := snapshotConfig()
		rec := c.RecentFolders
		if rec == nil {
			rec = []string{}
		}
		writeJSON(w, 200, map[string]any{"pending": takeQuickDeploy(), "recent": rec})
	})
	mux.HandleFunc("POST /api/quick/clear", func(w http.ResponseWriter, r *http.Request) {
		quick.Lock()
		quick.path = ""
		quick.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true})
	})
}

// deployRecOr404 resolves {slug}/{id} to a saved deploy or writes an error.
func deployRecOr404(w http.ResponseWriter, r *http.Request) (string, *Site, *deployIndex, *DeployRecord, bool) {
	id := r.PathValue("id")
	if !deployIDRe.MatchString(id) {
		writeErr(w, errors.New("invalid deploy id"))
		return "", nil, nil, nil, false
	}
	dir, s, err := projectDir(r.PathValue("slug"))
	if err != nil {
		writeErr(w, err)
		return "", nil, nil, nil, false
	}
	ix, err := readIndex(dir)
	if err != nil {
		writeErr(w, err)
		return "", nil, nil, nil, false
	}
	rec := ix.find(id)
	if rec == nil {
		writeJSON(w, 404, map[string]any{"error": "deploy not found in the history"})
		return "", nil, nil, nil, false
	}
	return dir, s, ix, rec, true
}

// noteSource remembers the folder on this PC a deploy came from, for
// "Redeploy last folder" (not the project's own current folder).
func noteSource(slug, p string) {
	if p == "" || !filepath.IsAbs(p) {
		return
	}
	if dir, _, err := projectDir(slug); err == nil && strings.HasPrefix(strings.ToLower(filepath.Clean(p)), strings.ToLower(filepath.Clean(dir))) {
		return
	}
	_ = withConfig(func(c *Config) error {
		if s := findSite(c, slug); s != nil {
			s.LastSource = p
		}
		return nil
	})
	rememberFolder(p)
}

func alertFailOn(slug string) bool {
	s, err := siteOr404(slug)
	return err != nil || boolOn(s.AlertFail)
}

// desktopNotify flashes the taskbar button when a deploy finishes while
// Rover is in the background (the UI also shows a system notification).
func desktopNotify(slug string, err error) { flashNativeWindow() }
