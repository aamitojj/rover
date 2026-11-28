package main

// Rover 0.9 Integration Centre: connect, test, edit and disconnect
// accounts for every provider in the modular catalog. Credentials are
// DPAPI-protected and never returned to the UI. Existing HostingAccounts,
// GitAccounts and the Cloudflare token stay in place; they also appear
// here so older settings keep working.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"hostpanel/prov"
)

// Integration is one connected account in the Integration Centre.
// Several accounts per provider are allowed. Cred is protected JSON
// (protectString / DPAPI) and is never sent to the UI or to Jiv.
type Integration struct {
	ID       string         `json:"id"`
	Provider string         `json:"provider"`
	Nickname string         `json:"nickname"`
	Auth     string         `json:"auth,omitempty"`
	Cred     string         `json:"cred"`
	Hint     string         `json:"hint,omitempty"`
	Who      string         `json:"who,omitempty"`
	Added    string         `json:"added"`
	Checked  string         `json:"checked,omitempty"`
	Status   string         `json:"status,omitempty"` // connected | disconnected | failed | unsupported
	Error    string         `json:"error,omitempty"`
	Plan     string         `json:"plan,omitempty"`
	Health   string         `json:"health,omitempty"`
	Usage    map[string]any `json:"usage,omitempty"`
}

func findIntegration(c *Config, id string) *Integration {
	for _, a := range c.Integrations {
		if a != nil && a.ID == id {
			return a
		}
	}
	return nil
}

func integCred(a *Integration) map[string]string {
	m := map[string]string{}
	if a == nil || a.Cred == "" {
		return m
	}
	_ = json.Unmarshal([]byte(secret(a.Cred)), &m)
	return m
}

func integHint(m MetaFields, cr map[string]string) string {
	for _, f := range m {
		if f.Secret && cr[f.Key] != "" {
			return keyHint(cr[f.Key])
		}
	}
	for _, k := range []string{"token", "access_key", "client_id", "json"} {
		if cr[k] != "" {
			return keyHint(cr[k])
		}
	}
	return ""
}

// MetaFields is a thin alias so integHint can accept prov fields without
// importing the Field type at every call site.
type MetaFields []prov.Field

type integView struct {
	Integration
	Cred         string            `json:"cred,omitempty"` // always empty
	Name         string            `json:"provider_name"`
	Capabilities map[string]string `json:"capabilities"`
	Scopes       []string          `json:"scopes,omitempty"`
	Docs         string            `json:"docs,omitempty"`
	Notes        string            `json:"notes,omitempty"`
	PaidWarning  string            `json:"paid_warning,omitempty"`
	Source       string            `json:"source,omitempty"` // integration | hosting | git | cloudflare | coolify | local
	Readonly     bool              `json:"readonly,omitempty"`
}

func integViews(c *Config) []integView {
	var out []integView
	seen := map[string]bool{}
	for _, a := range c.Integrations {
		if a == nil {
			continue
		}
		out = append(out, viewFromIntegration(a, "integration", false))
		seen[a.Provider+"|"+a.ID] = true
	}
	// Surface older account stores so nothing disappears after the upgrade.
	for _, a := range c.HostingAccounts {
		id := "hosting:" + a.ID
		if seen["hosting|"+id] {
			continue
		}
		st := a.Status
		if st == "ok" {
			st = "connected"
		} else if st == "error" {
			st = "failed"
		} else if st == "" {
			st = "connected"
		}
		v := integView{Integration: Integration{ID: id, Provider: a.Provider, Nickname: firstNonEmpty(a.Nickname, a.Who, a.Provider), Auth: a.Auth, Hint: a.Hint, Who: a.Who, Added: a.Added, Checked: a.Checked, Status: st, Error: a.Error}, Source: "hosting", Readonly: false}
		v.Cred = ""
		fillProvMeta(&v)
		out = append(out, v)
	}
	for host, g := range c.GitAccounts {
		id := "git:" + host
		provID := "github"
		if strings.Contains(host, "gitlab") {
			provID = "gitlab"
		} else if host == "netlify" {
			provID = "netlify"
		} else if !strings.Contains(host, "github") {
			continue
		}
		v := integView{Integration: Integration{ID: id, Provider: provID, Nickname: firstNonEmpty(g.Hint, host), Hint: g.Hint, Status: "connected"}, Source: "git"}
		fillProvMeta(&v)
		out = append(out, v)
	}
	if c.CloudflareToken != "" {
		v := integView{Integration: Integration{ID: "cloudflare:legacy", Provider: "cloudflare", Nickname: "Cloudflare DNS", Status: "connected"}, Source: "cloudflare"}
		fillProvMeta(&v)
		out = append(out, v)
	}
	if c.CoolifyToken != "" {
		v := integView{Integration: Integration{ID: "coolify:active", Provider: "coolify", Nickname: "Coolify on the active server", Who: c.CoolifyURL, Status: "connected"}, Source: "coolify", Readonly: true}
		fillProvMeta(&v)
		out = append(out, v)
	}
	if c.ThisPC != nil && c.ThisPC.Enabled {
		v := integView{Integration: Integration{ID: "local:thispc", Provider: "local", Nickname: "This PC", Status: "connected"}, Source: "local", Readonly: true}
		fillProvMeta(&v)
		out = append(out, v)
	}
	return out
}

func viewFromIntegration(a *Integration, source string, ro bool) integView {
	cp := *a
	cp.Cred = ""
	v := integView{Integration: cp, Source: source, Readonly: ro}
	fillProvMeta(&v)
	return v
}

func fillProvMeta(v *integView) {
	if p, ok := prov.Lookup(v.Provider); ok {
		m := p.Meta()
		v.Name = m.Name
		v.Docs = m.Docs
		v.Notes = m.Notes
		v.Scopes = m.Scopes
		v.PaidWarning = m.PaidWarning
		v.Capabilities = map[string]string{}
		for c, s := range m.Capabilities {
			v.Capabilities[string(c)] = string(s)
		}
	} else if hp, ok := hostingProviderByID(v.Provider); ok {
		v.Name = hp.Name
		v.Docs = hp.Docs
		v.Capabilities = map[string]string{"hosting": "supported", "cost": "partial"}
	} else {
		v.Name = v.Provider
	}
}

func catalogViews() []map[string]any {
	var out []map[string]any
	for _, m := range prov.Catalog() {
		caps := map[string]string{}
		for c, s := range m.Capabilities {
			caps[string(c)] = string(s)
		}
		out = append(out, map[string]any{
			"id": m.ID, "name": m.Name, "auth": m.Auth, "docs": m.Docs,
			"capabilities": caps, "fields": m.Fields, "scopes": m.Scopes,
			"notes": m.Notes, "existing_engine": m.Existing, "optional": m.Optional,
			"paid_warning": m.PaidWarning,
		})
	}
	return out
}

func validateIntegCred(p prov.Provider, in map[string]string) (map[string]string, error) {
	m := p.Meta()
	out := map[string]string{}
	for _, f := range m.Fields {
		v := in[f.Key]
		if !f.Multiline {
			v = strings.TrimSpace(v)
		}
		if v == "" && !f.Optional {
			// OAuth providers may have only access_token after sign-in.
			if m.Auth == "oauth" || m.Auth == "local" || m.Auth == "ssh" {
				continue
			}
			return nil, fmt.Errorf("please fill in %s", strings.ToLower(f.Label))
		}
		if len(v) > 100_000 {
			return nil, fmt.Errorf("%s is too long", f.Label)
		}
		out[f.Key] = v
	}
	for k, v := range in {
		if _, ok := out[k]; !ok && v != "" {
			out[k] = v
		}
	}
	return out, nil
}

func testIntegration(ctx context.Context, provider string, cr map[string]string) (*prov.Info, error) {
	p, ok := prov.Lookup(provider)
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", provider)
	}
	info, err := p.Test(ctx, integTransport(), cr)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// integTransport is the live transport (official API bases). Tests may
// replace it with one that points at httptest servers.
var integTransport = func() *prov.Transport { return &prov.Transport{} }

func applyTestInfo(a *Integration, info *prov.Info, err error) {
	a.Checked = time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		a.Status, a.Error, a.Health = "failed", err.Error(), "degraded"
		return
	}
	a.Status, a.Error = "connected", ""
	if info != nil {
		if info.Who != "" {
			a.Who = info.Who
		}
		a.Plan, a.Health, a.Usage = info.Plan, firstNonEmpty(info.Health, "ok"), info.Usage
	}
}

func registerIntegRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/integrations/catalog", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, catalogViews())
	})
	mux.HandleFunc("GET /api/integrations", func(w http.ResponseWriter, r *http.Request) {
		c := snapshotConfig()
		writeJSON(w, 200, integViews(&c))
	})
	mux.HandleFunc("POST /api/integrations", apiConnectIntegration)
	mux.HandleFunc("POST /api/integrations/{id}/test", apiTestIntegration)
	mux.HandleFunc("POST /api/integrations/{id}/cost", apiIntegrationCost)
	mux.HandleFunc("POST /api/integrations/{id}", apiEditIntegration)
	mux.HandleFunc("DELETE /api/integrations/{id}", apiDisconnectIntegration)
	mux.HandleFunc("GET /api/sites/{slug}/services", apiGetSiteServices)
	mux.HandleFunc("POST /api/sites/{slug}/services", apiSetSiteServices)
}

func apiConnectIntegration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider    string            `json:"provider"`
		Nickname    string            `json:"nickname"`
		Auth        string            `json:"auth"`
		Cred        map[string]string `json:"cred"`
		ConfirmPaid bool              `json:"confirm_paid"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	p, ok := prov.Lookup(req.Provider)
	if !ok {
		writeErr(w, fmt.Errorf("unknown provider %q", req.Provider))
		return
	}
	m := p.Meta()
	if m.Auth == "ssh" || m.ID == "nginx" {
		writeErr(w, errors.New("nginx direct is chosen as the engine on a server, not connected as a token here"))
		return
	}
	if m.ID == "local" || m.ID == "rover-analytics" {
		writeErr(w, errors.New("this is built into Rover and does not need a separate account"))
		return
	}
	cr, err := validateIntegCred(p, req.Cred)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	info, err := testIntegration(ctx, req.Provider, cr)
	if err != nil {
		writeErr(w, err)
		return
	}
	enc, err := protectCred(cr)
	if err != nil {
		writeErr(w, err)
		return
	}
	id := "int_" + randHex(8)
	nick := strings.TrimSpace(req.Nickname)
	if nick == "" {
		nick = firstNonEmpty(info.Who, m.Name)
	}
	a := &Integration{ID: id, Provider: req.Provider, Nickname: nick, Auth: firstNonEmpty(req.Auth, m.Auth), Cred: enc, Hint: integHint(m.Fields, cr), Added: time.Now().UTC().Format(time.RFC3339)}
	applyTestInfo(a, info, nil)
	if err := withConfig(func(c *Config) error {
		c.Integrations = append(c.Integrations, a)
		return nil
	}); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, viewFromIntegration(a, "integration", false))
}

func apiTestIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c := snapshotConfig()
	a := findIntegration(&c, id)
	if a == nil {
		writeErr(w, errors.New("unknown account"))
		return
	}
	cr := integCred(a)
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	info, err := testIntegration(ctx, a.Provider, cr)
	_ = withConfig(func(c *Config) error {
		if x := findIntegration(c, id); x != nil {
			applyTestInfo(x, info, err)
			a = x
		}
		return nil
	})
	if err != nil {
		writeJSON(w, 200, viewFromIntegration(a, "integration", false)) // status=failed, no secret
		return
	}
	writeJSON(w, 200, viewFromIntegration(a, "integration", false))
}

func apiEditIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Nickname string            `json:"nickname"`
		Cred     map[string]string `json:"cred"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	c := snapshotConfig()
	a := findIntegration(&c, id)
	if a == nil {
		writeErr(w, errors.New("unknown account"))
		return
	}
	p, ok := prov.Lookup(a.Provider)
	if !ok {
		writeErr(w, fmt.Errorf("unknown provider %q", a.Provider))
		return
	}
	var info *prov.Info
	var testErr error
	if len(req.Cred) > 0 {
		cr, err := validateIntegCred(p, req.Cred)
		if err != nil {
			writeErr(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		info, testErr = testIntegration(ctx, a.Provider, cr)
		if testErr != nil {
			writeErr(w, testErr)
			return
		}
		enc, err := protectCred(cr)
		if err != nil {
			writeErr(w, err)
			return
		}
		a.Cred, a.Hint = enc, integHint(p.Meta().Fields, cr)
		applyTestInfo(a, info, nil)
	}
	if n := strings.TrimSpace(req.Nickname); n != "" {
		a.Nickname = n
	}
	if err := withConfig(func(c *Config) error {
		x := findIntegration(c, id)
		if x == nil {
			return errors.New("unknown account")
		}
		x.Nickname, x.Cred, x.Hint = a.Nickname, a.Cred, a.Hint
		if len(req.Cred) > 0 {
			applyTestInfo(x, info, nil)
		}
		a = x
		return nil
	}); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, viewFromIntegration(a, "integration", false))
}

func apiDisconnectIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := withConfig(func(c *Config) error {
		keep := c.Integrations[:0]
		found := false
		for _, a := range c.Integrations {
			if a != nil && a.ID == id {
				found = true
				continue
			}
			keep = append(keep, a)
		}
		if !found {
			return errors.New("unknown account")
		}
		c.Integrations = keep
		for _, s := range c.Sites {
			if s.Services == nil {
				continue
			}
			for cap, v := range s.Services {
				if v == id {
					delete(s.Services, cap)
				}
			}
		}
		return nil
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func apiIntegrationCost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConfirmPaid bool `json:"confirm_paid"`
	}
	_ = readJSON(r, &req)
	c := snapshotConfig()
	a := findIntegration(&c, r.PathValue("id"))
	if a == nil {
		writeErr(w, errors.New("unknown account"))
		return
	}
	p, ok := prov.Lookup(a.Provider)
	if !ok {
		writeErr(w, errors.New("unknown provider"))
		return
	}
	coster, ok := p.(prov.Coster)
	if !ok {
		writeJSON(w, 200, map[string]any{"unsupported": true, "status": "unsupported", "error": "this provider has no cost API Rover can call"})
		return
	}
	if p.Meta().PaidWarning != "" && !req.ConfirmPaid {
		writeErr(w, fmt.Errorf("%s Confirm to continue.", p.Meta().PaidWarning))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	rep, err := coster.Cost(ctx, integTransport(), integCred(a), req.ConfirmPaid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, rep)
}

func apiGetSiteServices(w http.ResponseWriter, r *http.Request) {
	c := snapshotConfig()
	s := findSite(&c, r.PathValue("slug"))
	if s == nil {
		writeErr(w, errors.New("unknown project"))
		return
	}
	if s.Services == nil {
		s.Services = map[string]string{}
	}
	writeJSON(w, 200, map[string]any{"services": s.Services, "engine": s.Engine})
}

func apiSetSiteServices(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Services map[string]string `json:"services"`
		Engine   string            `json:"engine"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	slug := r.PathValue("slug")
	err := withConfig(func(c *Config) error {
		s := findSite(c, slug)
		if s == nil {
			return errors.New("unknown project")
		}
		if req.Engine != "" {
			switch req.Engine {
			case "coolify", "nginx", "netlify", "vercel", "firebase", "thispc", "local":
				s.Engine = req.Engine
			default:
				return fmt.Errorf("unknown engine %q", req.Engine)
			}
		}
		if req.Services != nil {
			for cap, id := range req.Services {
				if id == "" {
					if s.Services != nil {
						delete(s.Services, cap)
					}
					continue
				}
				if findIntegration(c, id) == nil && !strings.HasPrefix(id, "hosting:") && !strings.HasPrefix(id, "git:") && !strings.HasPrefix(id, "cloudflare:") && !strings.HasPrefix(id, "coolify:") && !strings.HasPrefix(id, "local:") {
					return fmt.Errorf("unknown account %s", id)
				}
				if s.Services == nil {
					s.Services = map[string]string{}
				}
				s.Services[cap] = id
			}
		}
		return nil
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	c := snapshotConfig()
	s := findSite(&c, slug)
	writeJSON(w, 200, map[string]any{"services": s.Services, "engine": s.Engine})
}
