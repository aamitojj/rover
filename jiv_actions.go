package main

// Jiv's action registry. Every action is either Safe (read-only or just
// navigation: runs straight away) or needs a confirm click. State-changing
// actions only run on the server through POST /api/jiv/actions/{id}/run
// with confirmed=true, using an id Rover issued itself - a model can
// propose actions but never run them.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type JivAction struct {
	Def    ToolDef
	Safe   bool // runs without a confirm click
	Client bool // performed by the UI (navigation, opening wizards)
	Site   bool // needs a project
}

var jivPages = []string{"dashboard", "projects", "new-project", "templates", "git-import", "netlify-import", "deploys", "domains", "testing", "jiv", "hosting", "settings", "server", "connect-ai", "storage", "backups", "updates", "organise", "jiv-settings", "guide", "shortcuts"}

var jivActions = []JivAction{
	{Def: ToolDef{"open_page", "Open a screen of Rover.", obj(map[string]any{"page": enum(jivPages...)}, "page")}, Safe: true, Client: true},
	{Def: ToolDef{"open_project", "Open a project on one of its tabs.", obj(map[string]any{"site": str("project slug"), "tab": enum("overview", "files", "deploys", "domain", "logs", "settings")}, "site")}, Safe: true, Client: true, Site: true},
	{Def: ToolDef{"open_project_folder", "Open the project's folder on this PC in File Explorer.", obj(map[string]any{"site": str("project slug")}, "site")}, Safe: true, Site: true},
	{Def: ToolDef{"diagnose", "Find out why a project (or everything) is down, failed or not working. Read-only.", obj(map[string]any{"site": str("project slug, empty = all projects")}, "")}, Safe: true},
	{Def: ToolDef{"check_dns", "Check what the internet sees for the project's domain and explain what to change. Read-only.", obj(map[string]any{"site": str("project slug")}, "site")}, Safe: true, Site: true},
	{Def: ToolDef{"run_health", "Check now whether the sites answer (uptime + HTTPS). Read-only.", obj(map[string]any{"site": str("project slug, empty = all")}, "")}, Safe: true},
	{Def: ToolDef{"explain_logs", "Read the project's recent log and explain the important lines. Read-only.", obj(map[string]any{"site": str("project slug")}, "site")}, Safe: true, Site: true},
	{Def: ToolDef{"set_domain", "Open the domain wizard for a project with the domain filled in (the wizard explains each step).", obj(map[string]any{"site": str("project slug"), "domain": str("example.com")}, "site")}, Safe: true, Client: true, Site: true},
	{Def: ToolDef{"add_hosting_account", "Open Hosting accounts to connect a provider account.", obj(map[string]any{"provider": enum("hetzner", "digitalocean", "vultr", "linode", "lightsail", "oracle", "contabo", "hostinger", "scaleway", "upcloud", "other")}, "")}, Safe: true, Client: true},
	{Def: ToolDef{"install_brain", "Open Settings > Jiv to download Jiv's offline brain.", obj(map[string]any{}, "")}, Safe: true, Client: true},
	{Def: ToolDef{"setup_server", "Open the server setup (installs the hosting engine on the server).", obj(map[string]any{}, "")}, Safe: true, Client: true},
	{Def: ToolDef{"create_site", "Create a new project (site) on the active server.", obj(map[string]any{"name": str("Project name")}, "name")}},
	{Def: ToolDef{"deploy_folder", "Publish the files in the project's folder on this PC as a new deploy.", obj(map[string]any{"site": str("project slug")}, "site")}, Site: true},
	{Def: ToolDef{"redeploy_site", "Restart a site with its current files.", obj(map[string]any{"site": str("project slug")}, "site")}, Site: true},
	{Def: ToolDef{"pull_and_deploy", "Get the latest commit from the project's Git repository and deploy it.", obj(map[string]any{"site": str("project slug")}, "site")}, Site: true},
	{Def: ToolDef{"rollback_site", "Put the previous deploy of a project live again.", obj(map[string]any{"site": str("project slug")}, "site")}, Site: true},
	{Def: ToolDef{"auto_dns", "Create the DNS records at Cloudflare for the project's domain.", obj(map[string]any{"site": str("project slug")}, "site")}, Site: true},
	{Def: ToolDef{"toggle_auto_deploy", "Turn auto-deploy on new Git commits on or off.", obj(map[string]any{"site": str("project slug"), "on": map[string]any{"type": "boolean"}}, "site", "on")}, Site: true},
	{Def: ToolDef{"backup_now", "Back up all projects now to the backup folder.", obj(map[string]any{}, "")}},
	{Def: ToolDef{"switch_server", "Make another connected server the active one.", obj(map[string]any{"server": str("server id or name")}, "server")}},
	{Def: ToolDef{"install_ollama", "Download and install Ollama (another local AI app) on this PC.", obj(map[string]any{}, "")}, Client: true},
}

func jivToolDefs() []ToolDef {
	out := make([]ToolDef, len(jivActions))
	for i, a := range jivActions {
		out[i] = a.Def
	}
	return out
}

func jivAction(name string) *JivAction {
	for i := range jivActions {
		if jivActions[i].Def.Name == name {
			return &jivActions[i]
		}
	}
	return nil
}

// ActionCard is what the UI shows (and the id it must send back).
type ActionCard struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Args   map[string]any `json:"args"`
	Label  string         `json:"label"`
	Detail string         `json:"detail,omitempty"`
	Safe   bool           `json:"safe"`
	Client bool           `json:"client"`
	Undo   string         `json:"undo,omitempty"`
}

type pendingAction struct {
	card    ActionCard
	created time.Time
	used    bool
}

var (
	pendMu  sync.Mutex
	pending = map[string]*pendingAction{}
)

// resolveSiteArg accepts a slug, a name or a domain (case-insensitive).
func resolveSiteArg(c *Config, v string) *Site {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return nil
	}
	for _, s := range c.Sites {
		if s.Slug == v || strings.ToLower(s.Name) == v || (s.Domain != "" && strings.ToLower(s.Domain) == v) {
			return s
		}
	}
	for _, s := range c.Sites {
		if editDistance(s.Slug, v, 1) <= 1 && len(v) >= 4 {
			return s
		}
	}
	return nil
}

func resolveServerArg(c *Config, v string) *ServerProfile {
	v = strings.ToLower(strings.TrimSpace(v))
	for _, p := range c.Servers {
		if strings.ToLower(p.ID) == v || strings.ToLower(p.Label) == v || p.ServerIP == v {
			return p
		}
	}
	for _, p := range c.Servers {
		if v != "" && strings.Contains(strings.ToLower(p.Label), v) {
			return p
		}
	}
	return nil
}

// proposeAction validates a tool call and stores it for a later confirmed
// run. Unknown tools or projects are rejected.
func proposeAction(tc ToolCall) (*ActionCard, error) {
	a := jivAction(tc.Name)
	if a == nil {
		return nil, fmt.Errorf("unknown action %q", tc.Name)
	}
	if tc.Args == nil {
		tc.Args = map[string]any{}
	}
	args := map[string]any{}
	for k, v := range tc.Args {
		if _, ok := a.Def.Parameters["properties"].(map[string]any)[k]; ok {
			args[k] = v
		}
	}
	c := snapshotConfig()
	sarg := func(k string) string { s, _ := args[k].(string); return strings.TrimSpace(s) }
	detail := ""
	if a.Site || sarg("site") != "" {
		s := resolveSiteArg(&c, sarg("site"))
		if s == nil {
			if a.Site {
				return nil, fmt.Errorf("I couldn't find a project called %q", sarg("site"))
			}
		} else {
			args["site"] = s.Slug
			if !onActive(&c, s) {
				detail = fmt.Sprintf("%s is on %s; Rover will ask to switch first.", s.Name, serverLabel(&c, s.Server))
			}
		}
	}
	switch tc.Name {
	case "create_site":
		n := sarg("name")
		if n == "" || !validSlug(slugify(n)) {
			return nil, errors.New("please give the new project a name (letters, numbers and dashes)")
		}
		if findSite(&c, slugify(n)) != nil {
			return nil, fmt.Errorf("a project called %q already exists", n)
		}
		detail = fmt.Sprintf("Creates %q on %s. You get an https address straight away; then drop your files on it.", n, firstNonEmpty(serverLabel(&c, c.ActiveServer), "your server"))
	case "switch_server":
		p := resolveServerArg(&c, sarg("server"))
		if p == nil {
			return nil, fmt.Errorf("I couldn't find a server called %q", sarg("server"))
		}
		args["server"] = p.ID
		detail = "New projects, deploys and checks will use " + p.Label + " (" + p.ServerIP + "). Nothing is deleted."
	case "rollback_site":
		detail = "The previous deploy goes live again. The current one stays in the Deploys list, so you can switch back."
	case "deploy_folder":
		if s := findSite(&c, sarg("site")); s != nil {
			detail = "Publishes the files in " + filepath.Join(projectsRoot(&c), s.Folder, "current") + " as a new deploy. The current version is kept."
		}
	case "redeploy_site":
		detail = "Restarts the site's web server with the current files (about a minute of downtime at most)."
	case "pull_and_deploy":
		if s := findSite(&c, sarg("site")); s != nil && s.Git == nil {
			return nil, fmt.Errorf("%s is not linked to a Git repository", s.Name)
		}
	case "toggle_auto_deploy":
		if _, ok := args["on"].(bool); !ok {
			args["on"] = true
		}
	case "backup_now":
		b := backupSettings(&c)
		if b.Dir == "" {
			detail = "Choose a backup folder first in Settings > Backups & alerts."
		} else {
			detail = "Copies all projects to " + b.Dir + "."
		}
	case "set_domain":
		if d := sarg("domain"); d != "" {
			nd, err := normalizeDomain(d)
			if err != nil {
				return nil, err
			}
			args["domain"] = nd
		}
	}
	card := ActionCard{ID: randHex(8), Name: tc.Name, Args: args, Safe: a.Safe, Client: a.Client, Detail: detail}
	card.Label = actionLabel(ToolCall{Name: tc.Name, Args: args})
	pendMu.Lock()
	for id, p := range pending {
		if time.Since(p.created) > time.Hour {
			delete(pending, id)
		}
	}
	pending[card.ID] = &pendingAction{card: card, created: time.Now()}
	pendMu.Unlock()
	return &card, nil
}

// appInner is the router behind guard; Jiv calls the same handlers the UI
// uses (including the server gate) so behaviour is identical.
var appInner http.Handler

func innerCall(method, path string, body any) (int, map[string]any) {
	var rd *bytes.Reader
	ct := "application/json"
	switch b := body.(type) {
	case nil:
		rd = bytes.NewReader(nil)
	case *multipart.Writer:
		panic("use innerMultipart")
	default:
		bs, _ := json.Marshal(b)
		rd = bytes.NewReader(bs)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", ct)
	return innerDo(req)
}

func innerDo(req *http.Request) (int, map[string]any) {
	rec := httptest.NewRecorder()
	if appInner == nil {
		return 500, map[string]any{"error": "not ready"}
	}
	appInner.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func innerLocalDeploy(slug, path string) (int, map[string]any) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("local_path", path)
	mw.Close()
	req := httptest.NewRequest("POST", "/api/sites/"+slug+"/deploy", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return innerDo(req)
}

// runAction performs a stored action. Returns a JSON-able result.
func runAction(id string, confirmed bool) (int, map[string]any) {
	pendMu.Lock()
	p := pending[id]
	if p != nil && !p.card.Safe && !confirmed {
		pendMu.Unlock()
		return http.StatusPreconditionRequired, map[string]any{"error": "please confirm this action first", "need_confirm": true}
	}
	if p != nil && !p.card.Safe {
		if p.used {
			pendMu.Unlock()
			return http.StatusConflict, map[string]any{"error": "this action already ran; ask again to repeat it"}
		}
		p.used = true
	}
	pendMu.Unlock()
	if p == nil {
		return http.StatusNotFound, map[string]any{"error": "this action expired; ask Jiv again"}
	}
	card := p.card
	if card.Client {
		return 200, map[string]any{"client": true, "action": card}
	}
	site, _ := card.Args["site"].(string)
	c := snapshotConfig()
	switch card.Name {
	case "open_project_folder":
		return innerCall("POST", "/api/projects/"+site+"/open", map[string]any{"what": "folder"})
	case "diagnose":
		var d Diagnosis
		if s := findSite(&c, site); s != nil {
			d = diagnoseSite(&c, s, true)
		} else {
			d = diagnoseAll(&c)
		}
		return 200, map[string]any{"ok": true, "diagnosis": d, "message": d.Summary}
	case "check_dns":
		s := findSite(&c, site)
		if s == nil {
			return 404, map[string]any{"error": "unknown project"}
		}
		if s.Domain == "" {
			return 200, map[string]any{"ok": true, "message": s.Name + " has no custom domain yet. Open its Domain tab to add one.", "href": "#/project/" + s.Slug + "/domain"}
		}
		d := Diagnosis{Site: s.Slug, Name: s.Name, Findings: diagnoseDomain(&c, s, true)}
		if len(d.Findings) == 0 {
			d.Findings = []Finding{{Level: "ok", Title: s.Domain + " is live with HTTPS", Text: "Everything is connected."}}
		}
		d.Summary = summarize(d)
		return 200, map[string]any{"ok": true, "diagnosis": d, "message": d.Summary}
	case "run_health":
		var slugs []string
		if site != "" {
			slugs = []string{site}
		}
		res := runHealthFor(slugs)
		var f []Finding
		keys := make([]string, 0, len(res))
		for k := range res {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			h := res[k]
			if h.OK {
				f = append(f, Finding{Level: "ok", Title: k + " answers", Text: fmt.Sprintf("%s in %d ms.", h.URL, h.MS)})
			} else {
				t, x, kb := explainError(h.Error)
				f = append(f, Finding{Level: "fail", Title: k + ": " + t, Text: x, KB: kb, Fix: tcall("diagnose", map[string]any{"site": k}), FixLbl: "Diagnose"})
			}
		}
		d := Diagnosis{Name: "your sites", Findings: f}
		if len(f) == 0 {
			d.Findings = []Finding{{Level: "info", Title: "Nothing to check yet", Text: "No published projects on the active server."}}
		}
		d.Summary = summarize(d)
		return 200, map[string]any{"ok": true, "diagnosis": d, "message": d.Summary}
	case "explain_logs":
		s := findSite(&c, site)
		if s == nil {
			return 404, map[string]any{"error": "unknown project"}
		}
		if !onActive(&c, s) {
			return innerCall("GET", "/api/sites/"+site+"/logs", nil) // the gate explains
		}
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		raw, err := fetchSiteLog(ctx, s, 20000)
		if err != nil {
			t, x, _ := explainError(err.Error())
			return 400, map[string]any{"error": t + ". " + x}
		}
		d := Diagnosis{Site: site, Name: s.Name, Findings: explainLog(raw)}
		d.Summary = summarize(d)
		return 200, map[string]any{"ok": true, "diagnosis": d, "message": d.Summary}
	case "create_site":
		n, _ := card.Args["name"].(string)
		return innerCall("POST", "/api/sites", map[string]any{"name": n})
	case "deploy_folder":
		s := findSite(&c, site)
		if s == nil {
			return 404, map[string]any{"error": "unknown project"}
		}
		return innerLocalDeploy(site, filepath.Join(projectsRoot(&c), s.Folder, "current"))
	case "redeploy_site":
		code, out := innerCall("POST", "/api/sites/"+site+"/restart", nil)
		if code == 200 {
			out["message"] = "Restart queued for " + site + ". It takes about a minute."
		}
		return code, out
	case "pull_and_deploy":
		return innerCall("POST", "/api/sites/"+site+"/pull", map[string]any{})
	case "rollback_site":
		code, out := innerCall("GET", "/api/sites/"+site+"/releases", nil)
		if code != 200 {
			return code, out
		}
		var rels []string
		for _, r := range asList(out["releases"]) {
			rels = append(rels, fmt.Sprint(r))
		}
		cur := fmt.Sprint(out["current"])
		prev := ""
		for i, r := range rels {
			if r == cur && i+1 < len(rels) {
				prev = rels[i+1]
			}
		}
		if prev == "" {
			return 400, map[string]any{"error": "there is no earlier deploy on the server to roll back to; publish an older one from the Deploys tab"}
		}
		code, res := innerCall("POST", "/api/sites/"+site+"/rollback", map[string]any{"release": prev})
		if code == 200 {
			res["message"] = fmt.Sprintf("%s: deploy %s is live again.", site, prev)
			res["undo"] = map[string]any{"site": site, "release": cur}
		}
		return code, res
	case "auto_dns":
		s := findSite(&c, site)
		if s == nil || s.Domain == "" {
			return 400, map[string]any{"error": "add a domain in the project's Domain tab first", "href": "#/project/" + site + "/domain"}
		}
		return innerCall("POST", "/api/sites/"+site+"/auto-dns", map[string]any{"domain": s.Domain, "www": s.IncludeWWW})
	case "toggle_auto_deploy":
		on, _ := card.Args["on"].(bool)
		code, out := innerCall("POST", "/api/sites/"+site+"/git", map[string]any{"auto_deploy": on})
		if code == 200 {
			out["message"] = fmt.Sprintf("Auto-deploy is %s for %s.", map[bool]string{true: "on", false: "off"}[on], site)
		}
		return code, out
	case "backup_now":
		return innerCall("POST", "/api/backup/run", nil)
	case "switch_server":
		sid, _ := card.Args["server"].(string)
		if err := activateServer(sid); err != nil {
			return 400, map[string]any{"error": err.Error()}
		}
		c := snapshotConfig()
		return 200, map[string]any{"ok": true, "message": "Now using " + serverLabel(&c, sid) + ".", "reload": true}
	}
	return 400, map[string]any{"error": "Jiv can't run " + card.Name + " here"}
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}
