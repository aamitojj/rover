package main

// Rover 0.7 routes: projects on several servers, failover, home servers.

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func migrateV07(c *Config) {
	for _, p := range c.Servers {
		if p.Provider == "thispc" {
			p.Kind = "thispc"
		}
	}
	for _, s := range c.Sites {
		if len(s.Servers) > 0 && s.HAMode == "" {
			s.HAMode = "backup"
		}
	}
}

// applyDomainSpecial handles a main server without Coolify (lite or This
// PC). done=false means the normal Coolify path applies.
func applyDomainSpecial(s *Site) (bool, error) {
	c := snapshotConfig()
	p := findServer(&c, s.Server)
	if p == nil || (p.Engine != "lite" && p.Kind != "thispc") {
		return false, nil
	}
	if p.Engine == "lite" {
		r, err := runnerFor(&c, p)
		if err != nil {
			return true, err
		}
		if out, err := runOut(r, liteHostsScript(remoteSitesRoot+"/"+s.Slug, s.Slug, liteHosts(s, p))); err != nil {
			return true, fmt.Errorf("%v: %s", err, strings.TrimSpace(out))
		}
	}
	_ = withConfig(func(c *Config) error {
		if x := findSite(c, s.Slug); x != nil {
			x.DomainLive, x.PendingNS = true, nil
		}
		return nil
	})
	go applyDomainReplicas(s.Slug)
	return true, nil
}

// applyDomainReplicas gives the project's other servers the domain too.
func applyDomainReplicas(slug string) {
	defer func() { _ = recover() }()
	c := snapshotConfig()
	s := findSite(&c, slug)
	if s == nil || s.Domain == "" {
		return
	}
	for _, id := range siteServerIDs(s) {
		p := findServer(&c, id)
		if p == nil {
			continue
		}
		if id != s.Server {
			switch {
			case p.Kind == "thispc":
			case p.Engine == "lite":
				if r, err := runnerFor(&c, p); err == nil {
					_, _ = runOut(r, liteHostsScript(remoteSitesRoot+"/"+slug, slug, liteHosts(s, p)))
				}
			default:
				rep := replicaOf(s, id)
				if rep == nil || rep.AppUUID == "" {
					continue
				}
				if api, err := coolifyFor(&c, p); err == nil {
					domains := []string{}
					for _, h := range siteHosts(s) {
						domains = append(domains, "https://"+h)
					}
					if rep.InstantURL != "" {
						domains = append(domains, rep.InstantURL)
					}
					if api.SetDomains(rep.AppUUID, strings.Join(domains, ","), true) == nil {
						_ = api.Deploy(rep.AppUUID)
					}
				}
			}
		}
		if p.TunnelID != "" {
			_ = syncServerTunnelConfig(&c, id)
		}
	}
	if s.HAMode == "together" && s.TunnelID != "" {
		_ = refreshProjectTunnel(&c, s)
	}
}

// refreshProjectTunnel republishes the hostnames and the DNS of a project
// tunnel (no connector changes).
func refreshProjectTunnel(c *Config, s *Site) error {
	cf, err := cfFromConfig(c)
	if err != nil {
		return err
	}
	acc, err := cfAccount(c, cf)
	if err != nil {
		return err
	}
	if err := cf.PutTunnelConfig(acc, s.TunnelID, tunnelIngress("https://localhost:443", siteHosts(s))); err != nil {
		return err
	}
	if s.CFZoneID == "" {
		return nil
	}
	for _, h := range siteHosts(s) {
		if _, err := cfPoint(cf, s.CFZoneID, h, "CNAME", tunnelTarget(s.TunnelID)); err != nil {
			return err
		}
	}
	return nil
}

type addable struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Kind   string `json:"kind"`
	Engine string `json:"engine"`
	Ready  bool   `json:"ready"`
	Why    string `json:"why,omitempty"`
}

// addableServers lists servers that could be added to a project.
func addableServers(c *Config, s *Site) []addable {
	in := map[string]bool{}
	for _, id := range siteServerIDs(s) {
		in[id] = true
	}
	var out []addable
	for _, p := range c.Servers {
		if in[p.ID] {
			continue
		}
		a := addable{ID: p.ID, Label: p.Label, Kind: firstNonEmpty(p.Kind, "vps"), Engine: firstNonEmpty(p.Engine, "coolify"), Ready: true}
		switch {
		case p.Kind == "thispc":
			if !thisPCOn(c) {
				a.Ready, a.Why = false, "turn on hosting on This PC first"
			}
		case p.ID != c.ActiveServer && p.HostKey == "":
			a.Ready, a.Why = false, "connect to it once first (Hosting accounts > Use this server)"
		case p.ID != c.ActiveServer && p.SSHKeyPath == "" && c.ManagedKey == "":
			a.Ready, a.Why = false, "needs Rover's password-free login: switch to it once and click Remember this server"
		case p.Engine != "lite" && p.CoolifyToken == "" && !(p.ID == c.ActiveServer && c.CoolifyToken != ""):
			a.Ready, a.Why = false, "the hosting engine is not set up there yet"
		}
		out = append(out, a)
	}
	return out
}

func replicasPayload(slug string, check bool) (map[string]any, error) {
	c := snapshotConfig()
	s := findSite(&c, slug)
	if s == nil {
		return nil, errors.New("unknown project")
	}
	views := replicaViews(&c, s, check)
	fo := FailoverSettings{Auto: true, SwitchBack: true}
	if s.Failover != nil {
		fo = *s.Failover
	}
	fo.Active = failoverActive(s)
	canSwitch := ""
	if err := switchReady(&c, s); err != nil {
		canSwitch = err.Error()
	}
	out := map[string]any{
		"mode": s.HAMode, "servers": views, "failover": fo, "release": s.Release,
		"can_switch": canSwitch == "", "switch_note": canSwitch,
		"cloudflare": c.CloudflareToken != "", "domain": s.Domain, "domain_on_cf": s.CFZoneID != "",
		"addable": addableServers(&c, s), "tunnel_id": s.TunnelID,
		"health": srvHealthSnapshot(slug),
	}
	if check && s.HAMode == "together" && s.TunnelID != "" {
		if t, err := tunnelStatus(&c, s.TunnelID); err == nil && t != nil {
			out["tunnel"] = t
		} else if err != nil {
			out["tunnel_error"] = err.Error()
		}
	}
	if check {
		all := true
		for _, v := range views {
			if v.InSync == nil || !*v.InSync {
				all = false
			}
		}
		out["in_sync"] = all
	}
	return out, nil
}

func registerV07Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sites/{slug}/replicas", func(w http.ResponseWriter, r *http.Request) {
		out, err := replicasPayload(r.PathValue("slug"), r.URL.Query().Get("check") == "1")
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /api/sites/{slug}/servers", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		var req struct{ Server, Mode string }
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		if req.Mode != "together" && req.Mode != "backup" {
			req.Mode = "backup"
		}
		c := snapshotConfig()
		s := findSite(&c, slug)
		if s == nil || findServer(&c, req.Server) == nil {
			writeErr(w, errors.New("unknown project or server"))
			return
		}
		for _, a := range addableServers(&c, s) {
			if a.ID == req.Server && !a.Ready {
				writeErr(w, fmt.Errorf("%s: %s", a.Label, a.Why))
				return
			}
		}
		j := newJob("add-server", func(j *Job) error { return addServerJob(j, slug, req.Server, req.Mode) })
		writeJSON(w, 200, map[string]any{"job": j.ID})
	})
	mux.HandleFunc("POST /api/sites/{slug}/servers/{id}/remove", func(w http.ResponseWriter, r *http.Request) {
		if err := removeServerFromSite(r.PathValue("slug"), r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/sites/{slug}/servers/{id}/main", func(w http.ResponseWriter, r *http.Request) {
		c := snapshotConfig()
		if p := findServer(&c, r.PathValue("id")); p != nil && p.Kind == "thispc" {
			writeErr(w, errors.New("This PC can be a backup or extra server, but not the main one"))
			return
		}
		if err := makeMain(r.PathValue("slug"), r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/sites/{slug}/replicas/sync", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		var req struct{ Servers []string }
		_ = readJSON(r, &req)
		c := snapshotConfig()
		s := findSite(&c, slug)
		if s == nil {
			writeErr(w, errors.New("unknown project"))
			return
		}
		ids := req.Servers
		if len(ids) == 0 {
			for _, id := range siteServerIDs(s) {
				if rp := replicaOf(s, id); rp == nil || rp.Status != "live" || rp.Release != s.Release {
					ids = append(ids, id)
				}
			}
		}
		if len(ids) == 0 {
			writeJSON(w, 200, map[string]any{"ok": true, "nothing": true})
			return
		}
		j := newJob("sync-servers", func(j *Job) error {
			err := syncReplicaJob(j, slug, ids)
			if err == nil {
				markLiveRecordComplete(slug)
			}
			return err
		})
		writeJSON(w, 200, map[string]any{"job": j.ID})
	})
	mux.HandleFunc("POST /api/sites/{slug}/ha", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		var req struct {
			Mode       string
			Auto       *bool
			SwitchBack *bool `json:"switch_back"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		if req.Mode != "" && req.Mode != "together" && req.Mode != "backup" {
			writeErr(w, errors.New("mode must be together or backup"))
			return
		}
		changedMode := false
		err := withConfig(func(c *Config) error {
			s := findSite(c, slug)
			if s == nil {
				return errors.New("unknown project")
			}
			if req.Mode != "" && req.Mode != s.HAMode {
				s.HAMode, changedMode = req.Mode, true
			}
			if s.Failover == nil {
				s.Failover = &FailoverSettings{Auto: true, SwitchBack: true, Active: s.Server}
			}
			if req.Auto != nil {
				s.Failover.Auto = *req.Auto
			}
			if req.SwitchBack != nil {
				s.Failover.SwitchBack = *req.SwitchBack
			}
			return nil
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		c := snapshotConfig()
		s := findSite(&c, slug)
		if changedMode && multiServer(s) {
			j := newJob("ha-mode", func(j *Job) error {
				msg, err := ensureHA(j, &c, s)
				if msg != "" {
					j.Logf("%s", msg)
				}
				return err
			})
			writeJSON(w, 200, map[string]any{"job": j.ID})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/sites/{slug}/failover/switch", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		var req struct{ To string }
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		c := snapshotConfig()
		s := findSite(&c, slug)
		if s == nil {
			writeErr(w, errors.New("unknown project"))
			return
		}
		ok := false
		for _, id := range siteServerIDs(s) {
			ok = ok || id == req.To
		}
		if !ok {
			writeErr(w, errors.New("the project does not run on that server"))
			return
		}
		from := failoverActive(s)
		if err := failoverSwitch(&c, s, req.To); err != nil {
			writeErr(w, err)
			return
		}
		addFailoverEvent(slug, fmt.Sprintf("Switched by you from %s to %s.", serverLabel(&c, from), serverLabel(&c, req.To)))
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/sites/{slug}/failover/check", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		if err := checkNow(slug); err != nil {
			writeErr(w, err)
			return
		}
		out, err := replicasPayload(slug, true)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, out)
	})

	// Home servers.
	mux.HandleFunc("POST /api/home/add", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Label, Host, User string
			Port              int
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		id, err := addHomeServer(req.Label, req.Host, req.Port, req.User)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := activateServer(id); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"id": id, "private": isPrivateHost(req.Host)})
	})
	mux.HandleFunc("POST /api/home/setup", func(w http.ResponseWriter, r *http.Request) {
		var req HomeSetupOptions
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		if !sshS.connected() {
			writeErr(w, errNotConnected)
			return
		}
		j := newJob("home-setup", func(j *Job) error { return homeSetupJob(j, req) })
		writeJSON(w, 200, map[string]any{"job": j.ID})
	})
	mux.HandleFunc("GET /api/home/guide", func(w http.ResponseWriter, r *http.Request) {
		c := snapshotConfig()
		p := findServer(&c, r.URL.Query().Get("server"))
		if p == nil {
			p = &ServerProfile{}
		}
		writeJSON(w, 200, map[string]any{"portforward": portForwardGuide(p), "tradeoffs": homeTradeoffs, "private": isPrivateHost(p.ServerIP), "server": p.Label})
	})
	mux.HandleFunc("POST /api/servers/{id}/tunnel", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c := snapshotConfig()
		if findServer(&c, id) == nil {
			writeErr(w, errors.New("unknown server"))
			return
		}
		j := newJob("server-tunnel", func(j *Job) error { _, err := ensureServerTunnel(j, id); return err })
		writeJSON(w, 200, map[string]any{"job": j.ID})
	})
	mux.HandleFunc("POST /api/servers/{id}/ddns", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req struct{ Host string }
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		err := withConfig(func(c *Config) error {
			p := findServer(c, id)
			if p == nil {
				return errors.New("unknown server")
			}
			p.DDNSHost, p.Reach = strings.ToLower(strings.TrimSpace(req.Host)), "portforward"
			return nil
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
}

// markLiveRecordComplete turns a "partial" deploy into "published" once
// every server has the live release.
func markLiveRecordComplete(slug string) {
	c := snapshotConfig()
	s := findSite(&c, slug)
	if s == nil {
		return
	}
	for _, id := range siteServerIDs(s) {
		if rp := replicaOf(s, id); rp == nil || rp.Status != "live" || rp.Release != s.Release {
			return
		}
	}
	dir, _, err := projectDir(slug)
	if err != nil {
		return
	}
	projMu.Lock()
	defer projMu.Unlock()
	ix, err := readIndex(dir)
	if err != nil {
		return
	}
	for _, d := range ix.Deploys {
		if d.Release == s.Release && d.Status == "partial" {
			d.Status, d.Error = "published", ""
		}
	}
	_ = writeIndex(dir, ix)
}
