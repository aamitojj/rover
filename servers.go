package main

// Rover 0.5: several servers. The classic top-level Config fields always
// describe the ACTIVE server (so all older code keeps working); Config.Servers
// keeps a profile per server. Switching copies the active fields into its
// profile and loads the other one. Each project remembers its server, and
// requests that would touch a project on another server are refused with a
// "switch_server" hint so the UI can ask the user first.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ServerProfile is one server Rover can deploy to. Secrets are protected
// with protectString (DPAPI on Windows), exactly like the top-level fields.
type ServerProfile struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	AccountID        string `json:"account_id,omitempty"` // HostingAccount it came from
	ProviderServerID string `json:"provider_server_id,omitempty"`
	Provider         string `json:"provider,omitempty"`

	ServerIP        string `json:"server_ip"`
	SSHPort         int    `json:"ssh_port"`
	SSHUser         string `json:"ssh_user"`
	SSHKeyPath      string `json:"ssh_key_path,omitempty"`
	HostKey         string `json:"host_key,omitempty"`
	CoolifyURL      string `json:"coolify_url,omitempty"`
	CoolifyToken    string `json:"coolify_token,omitempty"` // protected
	APIViaSSH       bool   `json:"api_via_ssh,omitempty"`
	ServerUUID      string `json:"server_uuid,omitempty"`
	AdminEmail      string `json:"admin_email,omitempty"`
	AdminPassword   string `json:"admin_password,omitempty"` // protected
	AutoSecurityUpd bool   `json:"auto_security_updates,omitempty"`
	Added           string `json:"added,omitempty"`

	// v0.7: home-made servers.
	Kind        string `json:"kind,omitempty"`         // "" rented/VPS | "home" (Linux at home) | "thispc"
	Engine      string `json:"engine,omitempty"`       // "" Coolify | "lite" (plain nginx, for small machines)
	Arch        string `json:"arch,omitempty"`         // uname -m
	RAMMB       int    `json:"ram_mb,omitempty"`       //
	Reach       string `json:"reach,omitempty"`        // "" public IP | "tunnel" | "portforward"
	TunnelID    string `json:"tunnel_id,omitempty"`    // this server's own Cloudflare Tunnel
	TunnelToken string `json:"tunnel_token,omitempty"` // protected
	DDNSHost    string `json:"ddns_host,omitempty"`    // port forwarding: the dynamic DNS name
}

func newServerID() string { return "srv-" + randHex(4) }

func (p *ServerProfile) captureFrom(c *Config) {
	p.ServerIP, p.SSHPort, p.SSHUser, p.SSHKeyPath, p.HostKey = c.ServerIP, c.SSHPort, c.SSHUser, c.SSHKeyPath, c.HostKey
	p.CoolifyURL, p.CoolifyToken, p.APIViaSSH, p.ServerUUID = c.CoolifyURL, c.CoolifyToken, c.APIViaSSH, c.ServerUUID
	p.AdminEmail, p.AdminPassword, p.AutoSecurityUpd = c.AdminEmail, c.AdminPassword, c.AutoSecurityUpd
	if c.Provider != "" {
		p.Provider, p.ProviderServerID = c.Provider, c.ProviderServerID
	}
}

func (p *ServerProfile) applyTo(c *Config) {
	c.ServerIP, c.SSHPort, c.SSHUser, c.SSHKeyPath, c.HostKey = p.ServerIP, p.SSHPort, p.SSHUser, p.SSHKeyPath, p.HostKey
	if c.SSHPort == 0 {
		c.SSHPort = 22
	}
	if c.SSHUser == "" {
		c.SSHUser = "root"
	}
	c.CoolifyURL, c.CoolifyToken, c.APIViaSSH, c.ServerUUID = p.CoolifyURL, p.CoolifyToken, p.APIViaSSH, p.ServerUUID
	c.AdminEmail, c.AdminPassword, c.AutoSecurityUpd = p.AdminEmail, p.AdminPassword, p.AutoSecurityUpd
	c.Provider, c.ProviderServerID = p.Provider, p.ProviderServerID
}

func findServer(c *Config, id string) *ServerProfile {
	for _, p := range c.Servers {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func serverLabel(c *Config, id string) string {
	if p := findServer(c, id); p != nil {
		return p.Label
	}
	return "another server"
}

// migrateV05 turns the single 0.4 server into the first server profile and
// gives every project that server. Jiv settings get their defaults.
func migrateV05(c *Config) {
	if c.Jiv == nil {
		c.Jiv = defaultJivSettings()
	}
	syncActiveServer(c)
}

// syncActiveServer runs before every save (with cfgMu held): it keeps the
// active profile in step with the top-level fields, creates a profile the
// first time a server is entered, and starts a NEW profile when the user
// types a different server address (the old server keeps its own data).
func syncActiveServer(c *Config) {
	if c.ServerIP == "" {
		return
	}
	act := findServer(c, c.ActiveServer)
	if act != nil && act.ServerIP != "" && act.ServerIP != c.ServerIP {
		// A different server was entered: keep the old profile as it was and
		// do not carry the old server's hosting-engine login over.
		if c.CoolifyURL == act.CoolifyURL || c.CoolifyURL == "" {
			c.CoolifyURL = "http://" + c.ServerIP + ":8000"
		}
		if c.CoolifyToken == act.CoolifyToken {
			c.CoolifyToken, c.ServerUUID = "", ""
		}
		if c.AdminPassword == act.AdminPassword {
			c.AdminEmail, c.AdminPassword = "", ""
		}
		if c.Provider == act.Provider && c.ProviderServerID == act.ProviderServerID {
			c.Provider, c.ProviderServerID = "", ""
		}
		act = nil
	}
	if act == nil {
		for _, p := range c.Servers { // re-entering a known server re-activates it
			if p.ServerIP == c.ServerIP {
				act = p
				break
			}
		}
	}
	if act == nil {
		act = &ServerProfile{ID: newServerID(), Label: "Server " + c.ServerIP, Added: time.Now().UTC().Format(time.RFC3339)}
		if len(c.Servers) == 0 {
			act.Label = "Main server (" + c.ServerIP + ")"
		}
		c.Servers = append(c.Servers, act)
	}
	c.ActiveServer = act.ID
	act.captureFrom(c)
	for _, s := range c.Sites {
		if s.Server == "" {
			s.Server = act.ID
		}
	}
}

// onActive reports whether the project lives on the active server.
func onActive(c *Config, s *Site) bool {
	return s.Server == "" || c.ActiveServer == "" || s.Server == c.ActiveServer
}

// activateServer makes profile id the active server and tries to connect
// with Rover's own key (no password). It never deletes anything.
func activateServer(id string) error {
	err := withConfig(func(c *Config) error {
		p := findServer(c, id)
		if p == nil {
			return errors.New("that server is not in Rover any more")
		}
		if p.Kind == "thispc" {
			return errors.New("This PC is managed on its own page (Hosting accounts > This PC); it is used as an extra server for projects")
		}
		if act := findServer(c, c.ActiveServer); act != nil && c.ServerIP != "" {
			act.captureFrom(c)
		}
		p.applyTo(c)
		c.ActiveServer = p.ID
		return nil
	})
	if err != nil {
		return err
	}
	sshS.close()
	localMu.Lock()
	localAt = time.Time{}
	localMu.Unlock()
	return nil
}

// tryConnectManaged connects to the active server with Rover's own key.
func tryConnectManaged() error {
	_, _, err := connectSSH("", "")
	return err
}

// gatedSitePath matches requests that need the project's own server.
var gatedSitePath = regexp.MustCompile(`^/api/(?:sites/([^/]+)/(deploy|restart|rollback|pull|domain|domain/apply|domain/finish|auto-dns|registrar-dns|cloudflare-move|switch-nameservers|releases|logs)|projects/([^/]+)/deploys/[^/]+/publish|sites/([^/]+))$`)

// serverGate refuses work on a project whose server is not active, with a
// hint the UI turns into "Switch to <server> and continue?".
func serverGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := gatedSitePath.FindStringSubmatch(r.URL.Path)
		if m != nil {
			slug := firstNonEmpty(m[1], m[3], m[4])
			whole := m[4] != ""
			if !whole || r.Method == http.MethodDelete {
				c := snapshotConfig()
				if s := findSite(&c, slug); s != nil && !onActive(&c, s) && findServer(&c, s.Server) != nil {
					writeJSON(w, http.StatusConflict, map[string]any{
						"error":         fmt.Sprintf("%s is on %s. Switch Rover to that server first.", s.Name, serverLabel(&c, s.Server)),
						"switch_server": s.Server, "server_label": serverLabel(&c, s.Server),
					})
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

type serverView struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	IP        string `json:"ip"`
	User      string `json:"user"`
	Provider  string `json:"provider,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	Active    bool   `json:"active"`
	Engine    bool   `json:"engine"` // Coolify token saved
	Projects  int    `json:"projects"`
	Connected bool   `json:"connected"`
	Kind      string `json:"kind,omitempty"`   // v0.7: "" | home | thispc
	Lite      bool   `json:"lite,omitempty"`   // v0.7: plain nginx engine
	Tunnel    bool   `json:"tunnel,omitempty"` // v0.7: connected through a Cloudflare tunnel
}

func serverViews(c *Config) []serverView {
	var out []serverView
	for _, p := range c.Servers {
		v := serverView{ID: p.ID, Label: p.Label, IP: p.ServerIP, User: p.SSHUser, Provider: p.Provider, AccountID: p.AccountID, Active: p.ID == c.ActiveServer, Engine: p.CoolifyToken != "" || p.Engine == "lite", Kind: p.Kind, Lite: p.Engine == "lite", Tunnel: p.TunnelID != ""}
		if v.Active {
			v.Engine = c.CoolifyToken != "" || p.Engine == "lite"
			v.Connected = sshS.connected()
		}
		for _, s := range c.Sites {
			if s.Server == p.ID {
				v.Projects++
			}
		}
		out = append(out, v)
	}
	return out
}

func registerServerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/servers", func(w http.ResponseWriter, r *http.Request) {
		c := snapshotConfig()
		writeJSON(w, 200, map[string]any{"servers": serverViews(&c), "active": c.ActiveServer})
	})
	mux.HandleFunc("POST /api/servers/{id}/activate", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := activateServer(id); err != nil {
			writeErr(w, err)
			return
		}
		connected := tryConnectManaged() == nil
		c := snapshotConfig()
		writeJSON(w, 200, map[string]any{"ok": true, "connected": connected, "label": serverLabel(&c, id)})
	})
	mux.HandleFunc("POST /api/servers/{id}/rename", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Label string }
		_ = readJSON(r, &req)
		req.Label = strings.TrimSpace(req.Label)
		if req.Label == "" || len(req.Label) > 60 {
			writeErr(w, errors.New("enter a name (up to 60 letters)"))
			return
		}
		err := withConfig(func(c *Config) error {
			p := findServer(c, r.PathValue("id"))
			if p == nil {
				return errors.New("unknown server")
			}
			p.Label = req.Label
			return nil
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	// Forget a server profile. Only allowed when no project uses it and it is
	// not the active one. The server itself is not touched.
	mux.HandleFunc("DELETE /api/servers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		err := withConfig(func(c *Config) error {
			if id == c.ActiveServer {
				return errors.New("this is the server Rover uses right now; switch to another one first")
			}
			for _, s := range c.Sites {
				if s.Server == id {
					return fmt.Errorf("%s still lives on this server; move it first", s.Name)
				}
			}
			keep := c.Servers[:0]
			for _, p := range c.Servers {
				if p.ID != id {
					keep = append(keep, p)
				}
			}
			c.Servers = keep
			return nil
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	// Move a project to another server: re-creates it there and publishes
	// its live deploy from the copy on this PC. DNS must then point to the
	// new server; Rover shows the new records.
	mux.HandleFunc("POST /api/sites/{slug}/server", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		var req struct{ Server string }
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		c := snapshotConfig()
		s := findSite(&c, slug)
		if s == nil || findServer(&c, req.Server) == nil {
			writeErr(w, errors.New("unknown project or server"))
			return
		}
		if s.Server == req.Server {
			writeJSON(w, 200, map[string]any{"ok": true, "unchanged": true})
			return
		}
		j := newJob("move-site", func(j *Job) error { return moveSiteJob(j, slug, req.Server) })
		writeJSON(w, 200, map[string]any{"job": j.ID})
	})
}

func moveSiteJob(j *Job, slug, target string) error {
	j.SetSteps("Switching to the new server", "Creating the project there", "Publishing the live version", "Done")
	j.Step(0)
	if err := activateServer(target); err != nil {
		return err
	}
	if err := tryConnectManaged(); err != nil {
		return fmt.Errorf("could not connect to the new server with Rover's key (%v). Open Settings > Server, connect once with the password, then try again", err)
	}
	c := snapshotConfig()
	if c.CoolifyToken == "" {
		return errors.New("the hosting engine is not set up on that server yet. Open Settings > Server and click Set up, then try again")
	}
	s := findSite(&c, slug)
	if s == nil {
		return errors.New("project not found")
	}
	j.Step(1)
	if err := provisionSite(j, s.Name, slug, true); err != nil {
		return err
	}
	_ = withConfig(func(c *Config) error {
		if x := findSite(c, slug); x != nil {
			x.Server = target
			x.Release, x.LastDeploy = "", ""
			if x.Domain != "" {
				x.DomainLive, x.HTTPSActive = false, false
			}
		}
		return nil
	})
	j.Step(2)
	c = snapshotConfig()
	s = findSite(&c, slug)
	if d := lastDeployOf(&c, s); d != nil {
		j.Logf("Publishing deploy %s from this PC...", d.ID)
		if err := republishDeploy(j, s, d); err != nil {
			j.Logf("Could not publish it automatically (%v). Open the Deploys tab and click Publish.", err)
		}
	} else {
		j.Logf("No saved deploy yet; drop your files on the project to publish them.")
	}
	if s.Domain != "" {
		j.Logf("Important: point %s to the new server %s (A record). Rover keeps checking and turns HTTPS on when it works.", s.Domain, c.ServerIP)
	}
	j.Step(3)
	return nil
}

// republishDeploy publishes a saved deploy zip again (used after a move).
var republishDeploy = func(j *Job, s *Site, d *DeployRecord) error {
	return publishArchived(context.Background(), j, s, d)
}

// publishArchived uploads a saved deploy zip to the active server.
func publishArchived(_ context.Context, j *Job, s *Site, d *DeployRecord) error {
	c := snapshotConfig()
	zp := filepath.Join(projectsRoot(&c), s.Folder, "deploys", d.File)
	entries, cleanup, err := zipEntries(zp)
	if err != nil {
		return fmt.Errorf("cannot open the local archive: %w", err)
	}
	defer cleanup()
	ws, err := newWorkspace(entries, "archive "+d.ID)
	if err != nil {
		return err
	}
	defer ws.Close()
	ws.restoreOf = d.ID
	return deployWorkspace(j, s.Slug, ws)
}
