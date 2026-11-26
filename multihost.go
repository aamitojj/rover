package main

// Rover 0.7: one project on several servers at once.
//
// Site.Server stays the project's main server (all older code keeps using
// it); Site.Servers lists the extra ones. A deploy goes to every server with
// the same release name: first the files are sent everywhere, then each
// server switches atomically (releaseScript). A deploy is "live" only when
// every server switched; otherwise the project shows which server failed and
// offers Retry, which republishes the live release from the copy on this PC.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Replica is the state of a project on one of its servers.
type Replica struct {
	AppUUID     string `json:"app_uuid,omitempty"`
	ProjectUUID string `json:"project_uuid,omitempty"`
	InstantURL  string `json:"instant_url,omitempty"`
	Release     string `json:"release,omitempty"` // live release on that server, as far as Rover knows
	Status      string `json:"status,omitempty"`  // live | failed | setting-up | removed
	Error       string `json:"error,omitempty"`
	Updated     string `json:"updated,omitempty"`
}

// runner is how Rover reaches a server's shell.
type runner interface {
	Upload(local, remote string, progress func(done, total int64)) error
	Run(ctx context.Context, script string, out io.Writer) error
	User() string
}

type sshRunner struct{ st *sshState }

func (r sshRunner) Upload(l, rp string, p func(done, total int64)) error {
	return r.st.upload(l, rp, p)
}
func (r sshRunner) Run(ctx context.Context, s string, out io.Writer) error {
	return r.st.run(ctx, s, out)
}
func (r sshRunner) User() string { _, u, _, _ := r.st.get(); return u }

// serverTarget is one server of a project, ready to receive files.
type serverTarget struct {
	ID, Label string
	r         runner // nil for this PC
	root      string // folder holding the sites (/data/hostpanel/sites)
	lite      bool   // plain nginx on the host (no Coolify)
	app       string // the project's Coolify app on that server (container name filter)
	thisPC    bool
	hosts     []string // names the lite nginx answers to
}

func (t *serverTarget) base(slug string) string { return t.root + "/" + slug }

// siteServerIDs lists the project's servers, main server first.
func siteServerIDs(s *Site) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range append([]string{s.Server}, s.Servers...) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func multiServer(s *Site) bool { return s != nil && len(siteServerIDs(s)) > 1 }

// replicaOf returns the per-server state (nil if unknown).
func replicaOf(s *Site, id string) *Replica {
	if s.Replicas == nil {
		return nil
	}
	return s.Replicas[id]
}

func replicaApp(s *Site, id string) string {
	if id == s.Server {
		return s.AppUUID
	}
	if r := replicaOf(s, id); r != nil {
		return r.AppUUID
	}
	return ""
}

// ---------- SSH connections to servers that are not the active one ----------

var sshPool = struct {
	sync.Mutex
	m map[string]*sshState
}{m: map[string]*sshState{}}

// connectProfile opens (or reuses) an SSH connection to a saved server with
// Rover's own key. Passwords are never stored, so a server must have been
// connected once (Hosting accounts > Use this server) before Rover can
// reach it in the background.
var connectProfile = func(c *Config, p *ServerProfile) (*sshState, error) {
	sshPool.Lock()
	st := sshPool.m[p.ID]
	if st == nil {
		st = &sshState{}
		sshPool.m[p.ID] = st
	}
	sshPool.Unlock()
	if st.connected() {
		return st, nil
	}
	if p.HostKey == "" {
		return nil, fmt.Errorf("Rover has not connected to %s yet. Open Hosting accounts, click Use this server once, then try again", p.Label)
	}
	var signer ssh.Signer
	var err error
	if p.SSHKeyPath != "" {
		b, rerr := os.ReadFile(p.SSHKeyPath)
		if rerr != nil {
			return nil, fmt.Errorf("read SSH key for %s: %w", p.Label, rerr)
		}
		signer, err = ssh.ParsePrivateKey(b)
	} else if c.ManagedKey != "" {
		signer, err = ssh.ParsePrivateKey([]byte(secret(c.ManagedKey)))
	} else {
		err = errors.New("no saved key")
	}
	if err != nil {
		return nil, fmt.Errorf("%s needs a password-free login (Rover's key). Switch to it once and click Remember this server: %v", p.Label, err)
	}
	want, _, _, _, perr := ssh.ParseAuthorizedKey([]byte(p.HostKey))
	if perr != nil {
		return nil, fmt.Errorf("stored host key of %s is invalid", p.Label)
	}
	conf := &ssh.ClientConfig{
		User: firstNonEmpty(p.SSHUser, "root"),
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if !bytes.Equal(want.Marshal(), key.Marshal()) {
				return fmt.Errorf("SERVER HOST KEY CHANGED for %s (expected %s, got %s)", p.Label, ssh.FingerprintSHA256(want), ssh.FingerprintSHA256(key))
			}
			return nil
		},
		Timeout: 15 * time.Second,
	}
	port := p.SSHPort
	if port == 0 {
		port = 22
	}
	client, err := ssh.Dial("tcp", net.JoinHostPort(p.ServerIP, strconv.Itoa(port)), conf)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s (%s): %w", p.Label, p.ServerIP, err)
	}
	st.close()
	st.mu.Lock()
	st.client, st.user = client, conf.User
	st.mu.Unlock()
	go keepAliveOn(st, client)
	return st, nil
}

// runnerFor returns a shell on the given server (the active one uses the
// session the user opened).
func runnerFor(c *Config, p *ServerProfile) (runner, error) {
	if p.ID == c.ActiveServer {
		if !sshS.connected() {
			if c.ManagedKey != "" && c.HostKey != "" {
				_, _, _ = connectSSH("", "")
			}
			if !sshS.connected() {
				return nil, errNotConnected
			}
		}
		return sshRunner{sshS}, nil
	}
	st, err := connectProfile(c, p)
	if err != nil {
		return nil, err
	}
	return sshRunner{st}, nil
}

// openTarget prepares one server of a project for a deploy (tests replace it).
var openTarget = func(c *Config, s *Site, id string) (*serverTarget, error) {
	p := findServer(c, id)
	if p == nil {
		return nil, fmt.Errorf("server %q is not in Rover any more", id)
	}
	t := &serverTarget{ID: id, Label: p.Label, root: remoteSitesRoot, lite: p.Engine == "lite", app: replicaApp(s, id), hosts: liteHosts(s, p)}
	if p.Kind == "thispc" {
		t.thisPC = true
		return t, nil
	}
	r, err := runnerFor(c, p)
	if err != nil {
		return nil, err
	}
	t.r = r
	return t, nil
}

// prefixWriter adds "[server] " to every line written to a job log.
type prefixWriter struct {
	j      *Job
	prefix string
	buf    []byte
	mu     sync.Mutex
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.j.Logf("%s%s", w.prefix, strings.TrimRight(string(w.buf[:i]), "\r"))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

func (w *prefixWriter) flush() {
	if len(w.buf) > 0 {
		w.j.Logf("%s%s", w.prefix, string(w.buf))
		w.buf = nil
	}
}

func remoteTmpFor(r runner, slug string) string {
	if r != nil && r.User() == "root" {
		return fmt.Sprintf("/root/.dilreet-%s-%s.tar.gz", slug, randHex(4))
	}
	return fmt.Sprintf("/tmp/dilreet-%s-%s.tar.gz", slug, randHex(4))
}

// partialError reports a deploy that went live on some servers only.
type partialError struct {
	Live    []string          // labels
	Failed  []string          // server ids
	Labels  map[string]string // id -> label
	Reasons map[string]string // id -> reason
	Nothing bool              // no server changed
}

func (e *partialError) Error() string {
	var f []string
	for _, id := range e.Failed {
		f = append(f, fmt.Sprintf("%s (%s)", e.Labels[id], e.Reasons[id]))
	}
	if e.Nothing {
		return "Nothing was changed: no server could receive the files: " + strings.Join(f, "; ")
	}
	return fmt.Sprintf("Live on %s only. Not updated: %s. Open the project's Hosting tab and click Retry when the server is back.", strings.Join(e.Live, ", "), strings.Join(f, "; "))
}

func shortErr(err error) string {
	s := strings.TrimSpace(err.Error())
	if len(s) > 220 {
		s = s[:220] + "..."
	}
	return s
}

func releaseName() string { return time.Now().UTC().Format("20060102-150405") }

// deployToAll publishes one tarball on every server of a project.
func deployToAll(j *Job, slug, tarball string, files int, size int64, conf string) error {
	c := snapshotConfig()
	s := findSite(&c, slug)
	if s == nil {
		return errors.New("unknown site")
	}
	return deployToTargets(j, &c, s, siteServerIDs(s), tarball, files, size, conf, releaseName(), false)
}

// deployToTargets sends the files to the given servers (phase 1), then
// switches each server that received them (phase 2). resync re-publishes an
// existing release name (Retry).
func deployToTargets(j *Job, c *Config, s *Site, ids []string, tarball string, files int, size int64, conf, release string, resync bool) error {
	ctx := context.Background()
	type slot struct {
		id, label string
		t         *serverTarget
		tmp       string
		err       error
	}
	slots := make([]*slot, len(ids))
	for i, id := range ids {
		slots[i] = &slot{id: id, label: serverLabel(c, id)}
	}
	j.Logf("Sending %d files (%.1f MB) to %d server(s): %s", files, float64(size)/1e6, len(ids), strings.Join(labelsOf(slots, func(x *slot) string { return x.label }), ", "))
	j.SetProgress(10, "Sending files to every server")
	var wg sync.WaitGroup
	for _, sl := range slots {
		wg.Add(1)
		go func(sl *slot) {
			defer wg.Done()
			t, err := openTarget(c, s, sl.id)
			if err != nil {
				sl.err = err
				return
			}
			sl.t = t
			if t.thisPC {
				sl.tmp = tarball
				return
			}
			sl.tmp = remoteTmpFor(t.r, s.Slug)
			last := time.Time{}
			sl.err = t.r.Upload(tarball, sl.tmp, func(done, total int64) {
				if time.Since(last) > 3*time.Second || done == total {
					last = time.Now()
					j.Logf("[%s] sent %.1f / %.1f MB", sl.label, float64(done)/1e6, float64(total)/1e6)
				}
			})
		}(sl)
	}
	wg.Wait()
	pe := &partialError{Labels: map[string]string{}, Reasons: map[string]string{}}
	ready := 0
	for _, sl := range slots {
		pe.Labels[sl.id] = sl.label
		if sl.err != nil {
			j.Logf("[%s] could not receive the files: %v", sl.label, sl.err)
		} else {
			ready++
		}
	}
	if ready == 0 {
		for _, sl := range slots {
			pe.Failed = append(pe.Failed, sl.id)
			pe.Reasons[sl.id] = shortErr(sl.err)
		}
		pe.Nothing = true
		recordReplicas(s.Slug, slots2results(slots, func(x *slot) (string, error) { return x.id, x.err }), "", false)
		return pe
	}
	j.Logf("Switching every server to release %s...", release)
	j.SetProgress(80, "Switching the servers")
	for _, sl := range slots {
		if sl.err != nil {
			continue
		}
		wg.Add(1)
		go func(sl *slot) {
			defer wg.Done()
			sl.err = publishOnTarget(ctx, j, s, sl.t, sl.tmp, release, conf, resync)
		}(sl)
	}
	wg.Wait()
	results := slots2results(slots, func(x *slot) (string, error) { return x.id, x.err })
	anyLive := false
	for _, sl := range slots {
		if sl.err == nil {
			anyLive = true
			pe.Live = append(pe.Live, sl.label)
			j.Logf("[%s] live: release %s", sl.label, release)
		} else {
			pe.Failed = append(pe.Failed, sl.id)
			pe.Reasons[sl.id] = shortErr(sl.err)
		}
	}
	recordReplicas(s.Slug, results, release, anyLive && !resync)
	if len(pe.Failed) > 0 {
		return pe
	}
	j.Logf("Live on all %d servers.", len(slots))
	return nil
}

func labelsOf[T any](xs []T, f func(T) string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, f(x))
	}
	return out
}

type targetResult struct {
	id  string
	err error
}

func slots2results[T any](xs []T, f func(T) (string, error)) []targetResult {
	out := make([]targetResult, 0, len(xs))
	for _, x := range xs {
		id, err := f(x)
		out = append(out, targetResult{id, err})
	}
	return out
}

// recordReplicas saves each server's outcome; with setLive the project's
// release moves to the new one.
func recordReplicas(slug string, res []targetResult, release string, setLive bool) {
	now := time.Now().Format(time.RFC3339)
	_ = withConfig(func(c *Config) error {
		s := findSite(c, slug)
		if s == nil {
			return nil
		}
		if s.Replicas == nil {
			s.Replicas = map[string]*Replica{}
		}
		for _, r := range res {
			rp := s.Replicas[r.id]
			if rp == nil {
				rp = &Replica{}
				s.Replicas[r.id] = rp
			}
			rp.Updated = now
			if r.err == nil {
				rp.Status, rp.Error = "live", ""
				if release != "" {
					rp.Release = release
				}
			} else {
				rp.Status, rp.Error = "failed", shortErr(r.err)
			}
		}
		if setLive && release != "" {
			s.Release, s.LastDeploy = release, now
		}
		return nil
	})
}

// publishOnTarget unpacks and switches one server.
func publishOnTarget(ctx context.Context, j *Job, s *Site, t *serverTarget, tmp, release, conf string, resync bool) error {
	if t.thisPC {
		return localPublish(s.Slug, tmp, release, resync)
	}
	site := *s
	site.AppUUID = t.app
	opts := relOpts{resync: resync}
	if t.lite {
		opts.lite = &liteOpts{slug: s.Slug, hosts: t.hosts}
	}
	script := releaseScriptOpts(t.base(s.Slug), tmp, release, &site, conf, opts)
	w := &prefixWriter{j: j, prefix: "[" + t.Label + "] "}
	err := t.r.Run(ctx, script, w)
	w.flush()
	return err
}

// ---------- Retry / sync one server ----------

// liveRecordOf finds the saved deploy that is live now (by release name).
func liveRecordOf(c *Config, s *Site) *DeployRecord {
	if s.Folder == "" {
		return nil
	}
	ix, err := readIndex(projectFolder(c, s))
	if err != nil {
		return nil
	}
	for _, d := range ix.Deploys {
		if d.Release != "" && d.Release == s.Release {
			return d
		}
	}
	for _, d := range ix.Deploys {
		if d.Status == "published" || d.Status == "partial" {
			return d
		}
	}
	return nil
}

// syncReplicaJob republishes the live release on servers that missed it.
func syncReplicaJob(j *Job, slug string, ids []string) error {
	c := snapshotConfig()
	s := findSite(&c, slug)
	if s == nil {
		return errors.New("unknown project")
	}
	if s.Release == "" {
		return errors.New("this project has not been published yet: deploy it first")
	}
	d := liveRecordOf(&c, s)
	if d == nil {
		return errors.New("the live version is not saved on this PC, so Rover cannot send it again. Deploy the project once more instead")
	}
	tarball, n, size, conf, cleanup, err := tarballFromRecord(&c, s, d)
	if err != nil {
		return err
	}
	defer cleanup()
	j.Logf("Sending the live version (%s, deploy %s) to %d server(s)...", s.Release, d.ID, len(ids))
	err = deployToTargets(j, &c, s, ids, tarball, n, size, conf, s.Release, true)
	if err == nil {
		j.Logf("All selected servers now have release %s.", s.Release)
	}
	return err
}

// tarballFromRecord rebuilds the upload and nginx config from a saved deploy.
var tarballFromRecord = func(c *Config, s *Site, d *DeployRecord) (string, int, int64, string, func(), error) {
	zp := filepath.Join(projectsRoot(c), s.Folder, "deploys", d.File)
	entries, cleanup, err := zipEntries(zp)
	if err != nil {
		return "", 0, 0, "", func() {}, fmt.Errorf("cannot open the local copy %s: %w", d.File, err)
	}
	ws, err := newWorkspace(entries, "archive "+d.ID)
	if err != nil {
		cleanup()
		return "", 0, 0, "", func() {}, err
	}
	pub, err := dirEntries(ws.Publish)
	if err != nil {
		ws.Close()
		cleanup()
		return "", 0, 0, "", func() {}, err
	}
	tb, n, size, err := buildSiteTarball(pub)
	conf := generateNginxConf(ws.Rules, ws.Files)
	rememberRules(s.Slug, ws.Rules)
	done := func() { os.Remove(tb); ws.Close(); cleanup() }
	if err != nil {
		done()
		return "", 0, 0, "", func() {}, err
	}
	return tb, n, size, conf, done, nil
}

func projectFolder(c *Config, s *Site) string {
	return filepath.Join(projectsRoot(c), s.Folder)
}

// ---------- rollback on every server ----------

// activateAll switches every server of a multi-server project to release.
func activateAll(slug, release string) error {
	c := snapshotConfig()
	s := findSite(&c, slug)
	if s == nil {
		return errors.New("unknown site")
	}
	ids := siteServerIDs(s)
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			t, err := openTarget(&c, s, id)
			if err != nil {
				errs[i] = err
				return
			}
			if t.thisPC {
				errs[i] = localActivate(slug, release)
				return
			}
			out, err := runOut(t.r, activateScript(t.base(slug), release, t.app, t.lite, slug, t.hosts))
			if err != nil {
				errs[i] = fmt.Errorf("%v: %s", err, strings.TrimSpace(out))
			}
		}(i, id)
	}
	wg.Wait()
	var res []targetResult
	var bad []string
	for i, id := range ids {
		res = append(res, targetResult{id, errs[i]})
		if errs[i] != nil {
			bad = append(bad, fmt.Sprintf("%s: %s", serverLabel(&c, id), shortErr(errs[i])))
		}
	}
	recordReplicas(slug, res, release, len(bad) < len(ids))
	if len(bad) > 0 {
		return fmt.Errorf("switched on %d of %d servers. Not switched: %s", len(ids)-len(bad), len(ids), strings.Join(bad, "; "))
	}
	return nil
}

func runOut(r runner, script string) (string, error) {
	var b bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err := r.Run(ctx, script, &b)
	return b.String(), err
}

// ---------- status / sync check ----------

type replicaView struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Role     string `json:"role"` // main | backup | replica
	Kind     string `json:"kind"`
	Engine   string `json:"engine"`
	IP       string `json:"ip"`
	Reach    string `json:"reach"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
	Release  string `json:"release,omitempty"` // what Rover recorded
	Actual   string `json:"actual,omitempty"`  // what the server reports (sync check)
	InSync   *bool  `json:"in_sync,omitempty"` // nil = not checked
	CheckErr string `json:"check_err,omitempty"`
	Health   string `json:"health"` // up | down | unknown
	Serving  bool   `json:"serving"`
	Updated  string `json:"updated,omitempty"`
}

func replicaViews(c *Config, s *Site, check bool) []replicaView {
	ids := siteServerIDs(s)
	out := make([]replicaView, len(ids))
	active := failoverActive(s)
	var wg sync.WaitGroup
	for i, id := range ids {
		p := findServer(c, id)
		v := replicaView{ID: id, Label: serverLabel(c, id), Role: "backup", Health: healthOf(s.Slug, id)}
		if s.HAMode == "together" {
			v.Role = "replica"
		}
		if i == 0 {
			v.Role = "main"
		}
		if p != nil {
			v.Kind, v.Engine, v.IP, v.Reach = firstNonEmpty(p.Kind, "vps"), firstNonEmpty(p.Engine, "coolify"), p.ServerIP, p.Reach
		}
		if r := replicaOf(s, id); r != nil {
			v.Status, v.Error, v.Release, v.Updated = r.Status, r.Error, r.Release, r.Updated
		} else if i == 0 && s.Release != "" {
			v.Status, v.Release = "live", s.Release
		}
		v.Serving = s.HAMode == "together" || id == active || (s.HAMode == "" && i == 0)
		out[i] = v
		if check {
			wg.Add(1)
			go func(i int, id string) {
				defer wg.Done()
				rel, err := liveReleaseOn(c, s, id)
				if err != nil {
					out[i].CheckErr = shortErr(err)
					return
				}
				out[i].Actual = rel
				ok := rel != "" && rel == s.Release
				out[i].InSync = &ok
			}(i, id)
		}
	}
	wg.Wait()
	return out
}

// liveReleaseOn asks a server which release it serves (tests replace it).
var liveReleaseOn = func(c *Config, s *Site, id string) (string, error) {
	t, err := openTarget(c, s, id)
	if err != nil {
		return "", err
	}
	if t.thisPC {
		return localLiveRelease(s.Slug), nil
	}
	out, err := runOut(t.r, fmt.Sprintf(`readlink %s/html || true`, shellQuote(t.base(s.Slug))))
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(out), "releases/"), nil
}

// ---------- adding and removing servers ----------

// addServerJob sets the project up on another server and sends it the live
// version. The project stays online on its other servers the whole time.
func addServerJob(j *Job, slug, serverID, mode string) error {
	j.SetSteps("Preparing the server", "Creating the project there", "Sending the live version", "Turning on automatic failover", "Done")
	j.Step(0)
	c := snapshotConfig()
	s := findSite(&c, slug)
	p := findServer(&c, serverID)
	if s == nil || p == nil {
		return errors.New("unknown project or server")
	}
	for _, id := range siteServerIDs(s) {
		if id == serverID {
			return fmt.Errorf("%s already runs on %s", s.Name, p.Label)
		}
	}
	_ = withConfig(func(c *Config) error {
		x := findSite(c, slug)
		if x.Replicas == nil {
			x.Replicas = map[string]*Replica{}
		}
		x.Replicas[serverID] = &Replica{Status: "setting-up", Updated: time.Now().Format(time.RFC3339)}
		return nil
	})
	fail := func(err error) error {
		recordReplicas(slug, []targetResult{{serverID, err}}, "", false)
		return err
	}
	j.Step(1)
	rep, err := provisionReplica(j, &c, s, p)
	if err != nil {
		return fail(err)
	}
	err = withConfig(func(c *Config) error {
		x := findSite(c, slug)
		x.Servers = append(x.Servers, serverID)
		rep.Status = "setting-up"
		x.Replicas[serverID] = rep
		if x.HAMode == "" {
			x.HAMode = mode
		}
		if x.HAMode == "backup" && x.Failover == nil {
			x.Failover = &FailoverSettings{Auto: true, SwitchBack: true, Active: x.Server}
		}
		return nil
	})
	if err != nil {
		return err
	}
	j.Step(2)
	if s.Release != "" {
		if err := syncReplicaJob(j, slug, []string{serverID}); err != nil {
			return err
		}
	} else {
		recordReplicas(slug, []targetResult{{serverID, nil}}, "", false)
		j.Logf("The project has no published version yet; the next deploy goes to every server.")
	}
	j.Step(3)
	c = snapshotConfig()
	s = findSite(&c, slug)
	if msg, err := ensureHA(j, &c, s); err != nil {
		j.Logf("Automatic failover is not on yet: %v", err)
	} else if msg != "" {
		j.Logf("%s", msg)
	}
	j.Step(4)
	j.Logf("%s now runs on %d servers: %s.", s.Name, len(siteServerIDs(s)), strings.Join(labelsOf(siteServerIDs(s), func(id string) string { return serverLabel(&c, id) }), ", "))
	return nil
}

// provisionReplica creates the project's folder (and web server) on p.
var provisionReplica = func(j *Job, c *Config, s *Site, p *ServerProfile) (*Replica, error) {
	if p.Kind == "thispc" {
		if err := localEnsureSite(s.Slug); err != nil {
			return nil, err
		}
		j.Logf("This PC will serve %s with Rover's built-in web server.", s.Name)
		return &Replica{}, nil
	}
	r, err := runnerFor(c, p)
	if err != nil {
		return nil, err
	}
	base := remoteSitesRoot + "/" + s.Slug
	if p.Engine == "lite" {
		j.Logf("Setting up %s on %s (plain nginx)...", s.Name, p.Label)
		out, err := runOut(r, liteSiteScript(base, s.Slug, liteHosts(s, p)))
		if err != nil {
			return nil, fmt.Errorf("prepare %s: %v: %s", p.Label, err, strings.TrimSpace(out))
		}
		return &Replica{InstantURL: "http://" + sslipHost(s.Slug, p.ServerIP)}, nil
	}
	if p.CoolifyToken == "" {
		return nil, fmt.Errorf("the hosting engine is not set up on %s yet. Switch to it once and click Set up", p.Label)
	}
	out, err := runOut(r, sitePrepScript(base))
	if err != nil {
		return nil, fmt.Errorf("prepare folder on %s: %v: %s", p.Label, err, strings.TrimSpace(out))
	}
	api, err := coolifyFor(c, p)
	if err != nil {
		return nil, err
	}
	srvUUID := p.ServerUUID
	if srvUUID == "" {
		srv, err := api.Servers()
		if err != nil || len(srv) == 0 {
			return nil, fmt.Errorf("Coolify on %s has no server: %v", p.Label, err)
		}
		srvUUID = srv[0].UUID
	}
	rep := &Replica{}
	j.Logf("Creating %s in Coolify on %s...", s.Name, p.Label)
	if rep.ProjectUUID, err = api.CreateProject(s.Name, "Rover site: "+s.Slug+" (extra server)"); err != nil {
		return nil, err
	}
	if rep.AppUUID, err = api.CreateNginxApp(rep.ProjectUUID, srvUUID, s.Slug); err != nil {
		return nil, err
	}
	if err := api.AddDirectoryMount(rep.AppUUID, base, "/usr/share/nginx"); err != nil {
		return nil, err
	}
	if err := api.AddDirectoryMount(rep.AppUUID, base+"/nginx", "/etc/nginx/conf.d"); err != nil {
		j.Logf("WARNING: rules mount on %s: %v", p.Label, err)
	}
	domains := []string{}
	if p.ServerIP != "" && p.Kind != "home" {
		rep.InstantURL = "https://" + sslipHost(s.Slug, p.ServerIP)
		domains = append(domains, rep.InstantURL)
	}
	if s.Domain != "" {
		for _, h := range siteHosts(s) {
			domains = append(domains, "https://"+h)
		}
	}
	if len(domains) > 0 {
		if err := retry(context.Background(), 3, 2*time.Second, func() error { return api.SetDomains(rep.AppUUID, strings.Join(domains, ","), true) }); err != nil {
			j.Logf("Note: could not set the addresses on %s: %v", p.Label, err)
		}
	}
	if err := api.Deploy(rep.AppUUID); err != nil {
		return nil, err
	}
	return rep, nil
}

// sitePrepScript creates the folder layout a Coolify nginx app mounts.
func sitePrepScript(base string) string {
	delim := "HPCONF_" + randHex(8)
	return fmt.Sprintf(`set -e
BASE=%[1]s
mkdir -p "$BASE/releases/initial" "$BASE/nginx" "$BASE/nginx-releases"
cat > "$BASE/nginx-releases/_default.conf" <<'%[4]s'
%[3]s
%[4]s
[ -f "$BASE/nginx/default.conf" ] || cp "$BASE/nginx-releases/_default.conf" "$BASE/nginx/default.conf"
if [ ! -e "$BASE/html" ]; then
  cat > "$BASE/releases/initial/index.html" <<'HPEOF'
%[2]s
HPEOF
  ln -sfn releases/initial "$BASE/html"
fi
chmod -R a+rX "$BASE"
echo ok`, shellQuote(base), placeholderHTML, strings.TrimRight(defaultNginxConf(), "\n"), delim)
}

// coolifyFor returns a Coolify client for any saved server.
func coolifyFor(c *Config, p *ServerProfile) (*Coolify, error) {
	if p.ID == c.ActiveServer {
		return newCoolify()
	}
	var st *sshState
	if p.APIViaSSH {
		var err error
		if st, err = connectProfile(c, p); err != nil {
			return nil, err
		}
	}
	return coolifyClient(p.CoolifyURL, secret(p.CoolifyToken), p.APIViaSSH, st)
}

// removeServerFromSite forgets one extra server of a project. Its files and
// web server stay on that machine (Rover stops sending deploys there).
func removeServerFromSite(slug, id string) error {
	return withConfig(func(c *Config) error {
		s := findSite(c, slug)
		if s == nil {
			return errors.New("unknown project")
		}
		if id == s.Server {
			return errors.New("this is the project's main server. Make another server the main one first, or move the project")
		}
		keep := s.Servers[:0]
		found := false
		for _, x := range s.Servers {
			if x == id {
				found = true
				continue
			}
			keep = append(keep, x)
		}
		if !found {
			return errors.New("the project does not run on that server")
		}
		s.Servers = keep
		if r := s.Replicas[id]; r != nil {
			r.Status = "removed"
		}
		if s.Failover != nil && s.Failover.Active == id {
			s.Failover.Active = s.Server
		}
		if len(s.Servers) == 0 {
			s.HAMode = ""
		}
		return nil
	})
}

// makeMain swaps the main server of a project (both must already have it).
func makeMain(slug, id string) error {
	return withConfig(func(c *Config) error {
		s := findSite(c, slug)
		if s == nil {
			return errors.New("unknown project")
		}
		if id == s.Server {
			return nil
		}
		idx := -1
		for i, x := range s.Servers {
			if x == id {
				idx = i
			}
		}
		if idx < 0 {
			return errors.New("add that server to the project first")
		}
		if s.Replicas == nil {
			s.Replicas = map[string]*Replica{}
		}
		old := s.Server
		oldRep := s.Replicas[old]
		if oldRep == nil {
			oldRep = &Replica{Release: s.Release, Status: "live"}
		}
		oldRep.AppUUID, oldRep.ProjectUUID, oldRep.InstantURL = s.AppUUID, s.ProjectUUID, s.InstantURL
		s.Replicas[old] = oldRep
		nr := s.Replicas[id]
		if nr == nil {
			nr = &Replica{}
		}
		s.AppUUID, s.ProjectUUID, s.InstantURL = nr.AppUUID, nr.ProjectUUID, nr.InstantURL
		s.Servers[idx] = old
		s.Server = id
		sort.SliceStable(s.Servers, func(a, b int) bool { return false })
		return nil
	})
}
