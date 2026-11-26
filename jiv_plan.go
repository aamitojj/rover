package main

// Rover 0.8: Jiv's "Do it for me" mode. A request such as "put my bakery
// site on joesbakery.com with HTTPS and a backup on my home PC" becomes a
// plan: a checklist of steps built from Rover's own actions.
//
//   - Without a model the planner uses intent templates (planFromText).
//   - With the offline brain it can also ask the model for a plan; the reply
//     is grammar-constrained JSON (llama-server json_schema), so it always
//     parses and can only name tools that exist. Every step is validated
//     the same way as a template step, and missing prerequisites are added.
//
// Safety: a plan only runs after the user clicks Run (confirmed=true with
// the server-issued plan id). Risky steps (restarting services, cleaning a
// server, moving a project, renewing certificates) stop and need their own
// confirm click with a one-time token. Read-only steps never change
// anything. A failed step stops the plan and shows the cause, a fix and
// Retry / Skip buttons.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PlanStep struct {
	N       int            `json:"n"`
	Tool    string         `json:"tool"`
	Args    map[string]any `json:"args"`
	Label   string         `json:"label"`
	Detail  string         `json:"detail,omitempty"`
	Risky   bool           `json:"risky,omitempty"`
	Read    bool           `json:"read_only,omitempty"`
	State   string         `json:"state"` // pending | running | needs_confirm | waiting | done | failed | skipped
	Result  string         `json:"result,omitempty"`
	Error   string         `json:"error,omitempty"`
	Fix     *PlanFix       `json:"fix,omitempty"`
	Log     []string       `json:"log,omitempty"`
	Token   string         `json:"token,omitempty"` // one-time confirm token while needs_confirm
	Guide   string         `json:"guide,omitempty"` // walkthrough for a part the user must do
	Diag    *Diagnosis     `json:"diagnosis,omitempty"`
	Added   bool           `json:"added,omitempty"` // added by Jiv while running (e.g. a retry after a fix)
	Started string         `json:"started,omitempty"`
}

type PlanFix struct {
	Title string      `json:"title"`
	Text  string      `json:"text"`
	KB    string      `json:"kb,omitempty"`
	Guide string      `json:"guide,omitempty"`
	Card  *ActionCard `json:"card,omitempty"`
}

type Plan struct {
	ID      string      `json:"id"`
	Goal    string      `json:"goal"`
	Source  string      `json:"source"` // templates | model
	Site    string      `json:"site,omitempty"`
	Steps   []*PlanStep `json:"steps"`
	State   string      `json:"state"` // ready | running | waiting | failed | done | cancelled
	Created time.Time   `json:"created"`
	Notes   []string    `json:"notes,omitempty"`
	Auto    bool        `json:"auto,omitempty"` // read-only plan that started by itself
	newSite string
	memo    map[string]string
	cancel  context.CancelFunc
	ctx     context.Context
	running bool
}

// ---------- tools ----------

type planTool struct {
	Name   string
	Desc   string
	Params []string
	Risky  bool
	Read   bool
	Site   bool // needs a project
	Order  int  // dependency order (create before deploy before domain ...)
	label  func(a map[string]any) string
	detail func(c *Config, a map[string]any) string
}

func sa(a map[string]any, k string) string { s, _ := a[k].(string); return strings.TrimSpace(s) }

var planTools = []planTool{
	{Name: "create_project", Desc: "Create a new project (website).", Params: []string{"name"}, Order: 10,
		label: func(a map[string]any) string { return "Create the project " + q(sa(a, "name")) }},
	{Name: "deploy_folder", Desc: "Publish the files in the project's folder on this PC.", Params: []string{"site"}, Site: true, Order: 20,
		label: func(a map[string]any) string { return "Publish " + sa(a, "site") + "'s files" }},
	{Name: "host_on_this_pc", Desc: "Turn on hosting on this PC (built-in web server).", Order: 30,
		label: func(a map[string]any) string { return "Turn on hosting on this PC" },
		detail: func(c *Config, a map[string]any) string {
			return "Starts Rover's built-in web server and keeps this PC awake while it hosts."
		}},
	{Name: "add_backup_server", Desc: "Run the project on a second server as a backup (failover).", Params: []string{"site", "server"}, Site: true, Order: 40,
		label: func(a map[string]any) string {
			return "Add " + serverName(sa(a, "server")) + " as a backup for " + sa(a, "site")
		}},
	{Name: "set_domain", Desc: "Save the project's domain.", Params: []string{"site", "domain"}, Site: true, Order: 50,
		label: func(a map[string]any) string { return "Use " + sa(a, "domain") + " for " + sa(a, "site") }},
	{Name: "dns_records", Desc: "Point the domain to the server (Cloudflare or registrar API, or guided steps).", Params: []string{"site"}, Site: true, Order: 60,
		label: func(a map[string]any) string { return "Point the domain's DNS to the server" }},
	{Name: "wait_dns", Desc: "Wait until the domain points to the server.", Params: []string{"site"}, Site: true, Read: true, Order: 70,
		label: func(a map[string]any) string { return "Wait for DNS to reach the server" }},
	{Name: "enable_https", Desc: "Connect the domain and turn on HTTPS (free certificate).", Params: []string{"site"}, Site: true, Order: 80,
		label: func(a map[string]any) string { return "Connect the domain and turn on HTTPS" }},
	{Name: "rollback", Desc: "Put the previous deploy live again.", Params: []string{"site"}, Site: true, Order: 90,
		label: func(a map[string]any) string { return "Roll " + sa(a, "site") + " back to the previous deploy" }},
	{Name: "restart_site", Desc: "Restart a site's web server.", Params: []string{"site"}, Site: true, Order: 90,
		label: func(a map[string]any) string { return "Restart " + sa(a, "site") }},
	{Name: "diagnose", Desc: "Find out why a project (or everything) is down. Read-only.", Params: []string{"site"}, Read: true, Order: 5,
		label: func(a map[string]any) string { return "Check " + firstNonEmpty(sa(a, "site"), "all projects") }},
	{Name: "check_dns", Desc: "Diagnose the domain's DNS and HTTPS. Read-only.", Params: []string{"site"}, Site: true, Read: true, Order: 5,
		label: func(a map[string]any) string { return "Diagnose " + sa(a, "site") + "'s domain (DNS + HTTPS)" }},
	{Name: "explain_logs", Desc: "Read the project's log and explain the important lines. Read-only.", Params: []string{"site"}, Site: true, Read: true, Order: 5,
		label: func(a map[string]any) string { return "Read and explain " + sa(a, "site") + "'s log" }},
	{Name: "why_deploy_failed", Desc: "Find why the last deploy failed. Read-only.", Params: []string{"site"}, Site: true, Read: true, Order: 6,
		label: func(a map[string]any) string { return "Find why " + sa(a, "site") + "'s last deploy failed" }},
	{Name: "retry_deploy", Desc: "Publish the last deploy's files again.", Params: []string{"site"}, Site: true, Order: 95,
		label: func(a map[string]any) string { return "Publish " + sa(a, "site") + "'s last deploy again" }},
	{Name: "fix_web_config", Desc: "Rewrite the site's web server settings (redirects, headers) from Rover's copy and test them.", Params: []string{"site"}, Site: true, Risky: true, Order: 92,
		label: func(a map[string]any) string { return "Rebuild " + sa(a, "site") + "'s web server settings" },
		detail: func(c *Config, a map[string]any) string {
			return "Publishes the current files again with fresh settings; nginx tests them first and keeps the old ones if they fail."
		}},
	{Name: "disk_usage", Desc: "Show what fills the server's disk. Read-only.", Params: []string{"server"}, Read: true, Order: 5,
		label: func(a map[string]any) string { return "Check disk space on " + serverName(sa(a, "server")) }},
	{Name: "free_disk", Desc: "Free disk space on a server (old Docker images, build cache, old logs).", Params: []string{"server"}, Risky: true, Order: 93,
		label: func(a map[string]any) string { return "Free disk space on " + serverName(sa(a, "server")) },
		detail: func(c *Config, a map[string]any) string {
			return "Removes unused Docker images and build cache older than 3 days, shrinks system logs to 200 MB and clears package downloads. Running sites are not touched."
		}},
	{Name: "check_services", Desc: "Find stopped or stuck services on a server. Read-only.", Params: []string{"server"}, Read: true, Order: 5,
		label: func(a map[string]any) string { return "Look for stuck services on " + serverName(sa(a, "server")) }},
	{Name: "restart_service", Desc: "Restart a stuck service (web proxy, nginx, Coolify, Docker, tunnel).", Params: []string{"server", "service"}, Risky: true, Order: 94,
		label: func(a map[string]any) string {
			if s := sa(a, "service"); s != "" && s != "auto" {
				return "Restart " + serviceName(s) + " on " + serverName(sa(a, "server"))
			}
			return "Restart the stuck services on " + serverName(sa(a, "server"))
		}},
	{Name: "renew_ssl", Desc: "Check the HTTPS certificate and renew it if it is expiring or broken.", Params: []string{"site"}, Site: true, Risky: true, Order: 96,
		label: func(a map[string]any) string { return "Renew " + sa(a, "site") + "'s HTTPS certificate if needed" },
		detail: func(c *Config, a map[string]any) string {
			return "Restarts the web proxy so it requests a fresh certificate (a few seconds of downtime)."
		}},
	{Name: "move_project", Desc: "Move a project to another server (migration).", Params: []string{"site", "server"}, Site: true, Risky: true, Order: 97,
		label: func(a map[string]any) string { return "Move " + sa(a, "site") + " to " + serverName(sa(a, "server")) },
		detail: func(c *Config, a map[string]any) string {
			return "Creates the project on the new server and publishes the live version there. Update the domain's DNS afterwards (Jiv tells you how)."
		}},
	{Name: "backup_now", Desc: "Back up all projects to the backup folder.", Order: 98,
		label: func(a map[string]any) string { return "Back up all projects now" }},
}

func q(s string) string { return "\u201c" + s + "\u201d" }

func planToolByName(n string) *planTool {
	for i := range planTools {
		if planTools[i].Name == n {
			return &planTools[i]
		}
	}
	return nil
}

func serverName(id string) string {
	c := snapshotConfig()
	switch id {
	case "", "active":
		if p := findServer(&c, c.ActiveServer); p != nil {
			return p.Label
		}
		return "the server"
	case thisPCID:
		return "This PC"
	}
	if p := findServer(&c, id); p != nil {
		return p.Label
	}
	return id
}

func serviceName(s string) string {
	return map[string]string{"proxy": "the web proxy (HTTPS)", "nginx": "nginx", "coolify": "the hosting engine (Coolify)", "docker": "Docker", "tunnel": "the Cloudflare tunnel"}[s]
}

// ---------- template planner (no model) ----------

var (
	reSplit       = regexp.MustCompile(`\s*(?:,|;|\band then\b|\bthen\b|\band also\b|\balso\b|\bplus\b|\band\b|\bwith\b|\baur\b|\bphir\b)\s*`)
	reHTTPS       = regexp.MustCompile(`\b(https|ssl|padlock|certificate|secure)\b`)
	reBackupOn    = regexp.MustCompile(`\b(backup|back-up|failover|fail over|second server|spare server|standby)\b`)
	reThisPC      = regexp.MustCompile(`\b(this pc|my pc|home pc|my computer|this computer|my laptop|this laptop|windows pc|desktop)\b`)
	reHomeSrv     = regexp.MustCompile(`\b(home server|raspberry|pi at home|machine at home|server at home|home machine)\b`)
	reFreeDisk    = regexp.MustCompile(`\b(free|clean|clear|make)\b.*\b(disk|space|storage)\b|\bdisk ?full\b|\bdiskfull\b|\bno space\b|\bout of (disk|space)\b`)
	reDiskCheck   = regexp.MustCompile(`\b(what|why)\b.*\b(fill\w*|using|full|eating)\b.*\b(disk|space)\b|\bdisk usage\b`)
	reStuck       = regexp.MustCompile(`\b(stuck|hung|hang|frozen|not responding|restart)\b.*\b(service|services|nginx|proxy|traefik|docker|coolify|tunnel|server)\b|\brestart (the )?(stuck )?services?\b`)
	reRenewSSL    = regexp.MustCompile(`\b(renew|refresh|fix|expired?|expiring)\b.*\b(ssl|https|certificate|cert)\b|\b(ssl|https|certificate|cert)\b.*\b(expired?|expiring|renew)\b`)
	reMove        = regexp.MustCompile(`\b(move|migrate|transfer|shift)\b`)
	reWhyDeploy   = regexp.MustCompile(`\b(why|fix|find)\b.*\b(deploy|publish|upload)\w*\b.*\b(fail|error|broke)\w*|\b(deploy|publish|upload)\w* (failed|fails|failing|error)\b|\bfailed deploy\b`)
	reFixConfig   = regexp.MustCompile(`\b(nginx|redirect|redirects|rewrite|headers?|too many redirects|redirect loop)\b.*\b(error|fix|broken|loop|wrong|rejected)\b|\b(fix|repair)\b.*\b(nginx|redirects?|headers?)\b|\btoo many redirects\b`)
	reDNSDiag     = regexp.MustCompile(`\b(dns|nameserver|domain)\b.*\b(problem|issue|wrong|check|diagnos\w*|not work\w*|notwork|down)\b|\bdiagnos\w*\b.*\b(dns|domain)\b`)
	reLogExplain  = regexp.MustCompile(`\b(explain|read|check|understand|samjhao)\b.*\blogs?\b|\blogs?\b.*\b(mean|explain|samjhao)\b`)
	reRollbackP   = regexp.MustCompile(`\b(roll ?back|rollback|revert|restore (the )?(old|previous|last)|previous version|old version|undo (the )?(last )?deploy)\b`)
	reRestartSite = regexp.MustCompile(`\brestart\b`)
	reDeployP     = regexp.MustCompile(`\b(deploy|publish|upload|go live|make (it )?live|put (it )?live)\b`)
	reCreateP     = regexp.MustCompile(`\b(create|make|new|start)\b.*\b(project|site|website)\b`)
	reBackupNow   = regexp.MustCompile(`\b(back ?up|backup) (now|everything|all)\b|\b(take|make|run) a backup\b`)
	reDoIt        = regexp.MustCompile(`\b(do it|for me|automatically|set (it )?up|setup|put|make|kar do|kardo|karo|fix|free|clean|restart|renew|move|migrate)\b`)
)

var serviceWords = map[string]string{"nginx": "nginx", "proxy": "proxy", "traefik": "proxy", "coolify": "coolify", "docker": "docker", "tunnel": "tunnel", "cloudflared": "tunnel"}

type planDraft struct {
	steps    []*PlanStep
	needSite bool
	site     string
	notes    []string
}

func (d *planDraft) add(tool string, args map[string]any) {
	for _, s := range d.steps {
		if s.Tool == tool && fmt.Sprint(s.Args) == fmt.Sprint(args) {
			return
		}
	}
	d.steps = append(d.steps, &PlanStep{Tool: tool, Args: args})
}

func (d *planDraft) has(tool string) bool {
	for _, s := range d.steps {
		if s.Tool == tool {
			return true
		}
	}
	return false
}

// planFromText turns a request into steps with intent templates.
func planFromText(c *Config, text, curSite string) planDraft {
	var d planDraft
	raw := strings.TrimSpace(text)
	t := jivNormalize(raw)
	site := findSiteIn(c, reDomain.ReplaceAllString(strings.ToLower(raw), " ")) // "shop.com" is not project "shop"
	newName := ""
	if reCreateP.MatchString(t) {
		if m := reCalled.FindStringSubmatch(t); m != nil {
			newName = strings.TrimSpace(m[1])
			for _, stop := range []string{" and ", " with ", " on ", " then "} {
				if i := strings.Index(newName, stop); i > 0 {
					newName = newName[:i]
				}
			}
		}
	}
	slug := ""
	switch {
	case newName != "":
		slug = "$new"
	case site != nil:
		slug = site.Slug
	case curSite != "" && findSite(c, curSite) != nil:
		slug = curSite
	case len(activeSites(c)) == 1:
		slug = activeSites(c)[0].Slug
	}
	d.site = slug
	withSite := func(m map[string]any) map[string]any {
		if m == nil {
			m = map[string]any{}
		}
		m["site"] = slug
		if slug == "" {
			d.needSite = true
		}
		return m
	}
	serverIn := func(s string) string { return serverInText(c, s) }
	domain := ""
	for _, m := range reDomain.FindAllString(t, -1) {
		if !strings.HasSuffix(m, ".html") && !strings.HasSuffix(m, ".zip") && !isSiteDomainOther(c, m, slug) && !strings.Contains(m, "sslip") {
			domain = m
			break
		}
	}
	if newName != "" {
		d.add("create_project", map[string]any{"name": newName})
	}
	clauses := reSplit.Split(t, -1)
	full := " " + t + " "
	for _, cl := range append([]string{full}, clauses...) {
		cl = strings.TrimSpace(cl)
		if cl == "" {
			continue
		}
		switch {
		case reWhyDeploy.MatchString(cl):
			d.add("why_deploy_failed", withSite(nil))
		case reFixConfig.MatchString(cl):
			d.add("check_dns_or_logs", nil)
		case reFreeDisk.MatchString(cl):
			srv := serverIn(cl)
			d.add("disk_usage", map[string]any{"server": srv})
			d.add("free_disk", map[string]any{"server": srv})
		case reDiskCheck.MatchString(cl):
			d.add("disk_usage", map[string]any{"server": serverIn(cl)})
		case reRenewSSL.MatchString(cl):
			d.add("renew_ssl", withSite(nil))
		case reStuck.MatchString(cl):
			srv, svc := serverIn(cl), "auto"
			for w, s := range serviceWords {
				if strings.Contains(cl, w) {
					svc = s
				}
			}
			if svc == "auto" {
				d.add("check_services", map[string]any{"server": srv})
			}
			d.add("restart_service", map[string]any{"server": srv, "service": svc})
		case reMove.MatchString(cl) && serverIn(cl) != "" && !reBackupOn.MatchString(cl):
			d.add("move_project", withSite(map[string]any{"server": serverIn(cl)}))
		case reLogExplain.MatchString(cl):
			d.add("explain_logs", withSite(nil))
		case reDNSDiag.MatchString(cl) && domain == "":
			d.add("check_dns", withSite(nil))
		case reBackupNow.MatchString(cl):
			d.add("backup_now", map[string]any{})
		case reBackupOn.MatchString(cl):
			srv := serverIn(cl)
			switch {
			case reThisPC.MatchString(cl):
				srv = thisPCID
			case reHomeSrv.MatchString(cl):
				for _, p := range c.Servers {
					if p.Kind == "home" {
						srv = p.ID
					}
				}
				if srv == "" {
					d.notes = append(d.notes, "No home server is connected yet: add it first in Hosting accounts > Add a home server (Jiv can show you how).")
				}
			}
			if srv == "" {
				srv = firstAddable(c, slug)
			}
			if srv == thisPCID && (c.ThisPC == nil || !c.ThisPC.Enabled) {
				d.add("host_on_this_pc", map[string]any{})
			}
			if srv != "" {
				d.add("add_backup_server", withSite(map[string]any{"server": srv}))
			} else if !reHomeSrv.MatchString(cl) {
				d.notes = append(d.notes, "There is no second server to use as a backup yet. Connect one in Hosting accounts, add a home server, or turn on This PC.")
			}
		case reRollbackP.MatchString(cl):
			d.add("rollback", withSite(nil))
		case reHTTPS.MatchString(cl) && domain == "":
			d.add("enable_https", withSite(nil))
		case reDeployP.MatchString(cl) && domain == "" && !reBackupOn.MatchString(full):
			d.add("deploy_folder", withSite(nil))
		case reRestartSite.MatchString(cl) && !reStuck.MatchString(cl):
			d.add("restart_site", withSite(nil))
		}
	}
	if domain != "" && (reAddDomain.MatchString(t) || reDeployP.MatchString(t) || reHTTPS.MatchString(t) || strings.Contains(t, " on "+domain) || strings.Contains(t, "to "+domain)) {
		d.add("set_domain", withSite(map[string]any{"domain": domain}))
		d.add("dns_records", withSite(nil))
		d.add("wait_dns", withSite(nil))
		d.add("enable_https", withSite(nil))
	}
	if newName != "" && !d.has("deploy_folder") {
		d.notes = append(d.notes, "After the project is created, drop your site's files on it (Jiv's walkthrough shows where).")
	}
	// "fix nginx/redirect errors": read the log first, then rebuild the config
	for i, s := range d.steps {
		if s.Tool == "check_dns_or_logs" {
			d.steps[i] = &PlanStep{Tool: "explain_logs", Args: withSite(nil)}
			d.steps = append(d.steps, &PlanStep{Tool: "fix_web_config", Args: withSite(nil)})
		}
	}
	return d
}

func isSiteDomainOther(c *Config, d, slug string) bool {
	for _, s := range c.Sites {
		if s.Domain == d && s.Slug != slug {
			return false // used by another project: still a domain the user named; validation reports it
		}
	}
	return false
}

// firstAddable picks a ready second server for a backup.
func firstAddable(c *Config, slug string) string {
	s := findSite(c, slug)
	for _, p := range c.Servers {
		if p.Kind == "thispc" || (s != nil && (p.ID == s.Server || contains(s.Servers, p.ID))) {
			continue
		}
		if s == nil && p.ID == c.ActiveServer {
			continue
		}
		return p.ID
	}
	return ""
}

// ---------- model planner ----------

// serverInText finds a server named in the text (label, IP or id).
func serverInText(c *Config, s string) string {
	for _, p := range c.Servers {
		for _, k := range []string{strings.ToLower(p.Label), p.ServerIP, strings.ToLower(p.ID)} {
			if k != "" && len(k) >= 3 && strings.Contains(s, k) {
				return p.ID
			}
		}
		if w := strings.ToLower(strings.Fields(p.Label + " x")[0]); len(w) >= 4 && strings.Contains(s, w) {
			return p.ID
		}
	}
	return ""
}

// planSchemaFor is the grammar for model plans. Projects and servers are
// enums of what exists, so a small model cannot invent or garble them.
func planSchemaFor(c *Config) map[string]any {
	sch := planSchema()
	props := sch["properties"].(map[string]any)["steps"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	sites := []string{"$new"}
	for _, s := range activeSites(c) {
		sites = append(sites, s.Slug)
	}
	servers := []string{}
	seen := map[string]bool{}
	for _, p := range c.Servers {
		if !seen[p.ID] {
			servers = append(servers, p.ID)
			seen[p.ID] = true
		}
	}
	if !seen[thisPCID] {
		servers = append(servers, thisPCID)
	}
	props["site"] = map[string]any{"type": "string", "enum": sites}
	props["server"] = map[string]any{"type": "string", "enum": servers}
	props["domain"] = map[string]any{"type": "string", "pattern": "^[a-z0-9-]+([.][a-z0-9-]+)+$"}
	props["name"] = map[string]any{"type": "string", "maxLength": 40}
	return sch
}

func planSchema() map[string]any {
	var names []string
	for _, t := range planTools {
		names = append(names, t.Name)
	}
	return map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"steps"},
		"properties": map[string]any{"steps": map[string]any{"type": "array", "minItems": 1, "maxItems": 10, "items": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"tool"},
			"properties": map[string]any{
				"tool": map[string]any{"type": "string", "enum": names}, "site": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"},
				"domain": map[string]any{"type": "string"}, "server": map[string]any{"type": "string"},
				"service": map[string]any{"type": "string", "enum": []string{"auto", "proxy", "nginx", "coolify", "docker", "tunnel"}},
			}}}},
	}
}

func planFromModel(ctx context.Context, c *Config, text, curSite string) (planDraft, error) {
	var b strings.Builder
	b.WriteString("You plan tasks for Rover, a Windows app that publishes websites to the user's servers. Turn the request into the shortest list of tool steps, in order. Use only these tools:\n")
	for _, t := range planTools {
		fmt.Fprintf(&b, "- %s(%s): %s\n", t.Name, strings.Join(t.Params, ", "), t.Desc)
	}
	b.WriteString("Rules: a new domain needs set_domain, dns_records, wait_dns, enable_https. A backup on this PC is server \"this-pc\" and needs host_on_this_pc first. Diagnose (read-only tools) before fixing. Use project slugs and server ids from APP STATE. A new project in the same plan has site \"$new\".\n\nAPP STATE:\nProjects:")
	for _, s := range activeSites(c) {
		fmt.Fprintf(&b, " %s (%s%s)", s.Slug, s.Name, map[bool]string{true: ", domain " + s.Domain, false: ""}[s.Domain != ""])
	}
	b.WriteString("\nServers:")
	for _, p := range c.Servers {
		fmt.Fprintf(&b, " %s (%s, %s)", p.ID, p.Label, firstNonEmpty(p.Kind, "vps"))
	}
	if curSite != "" {
		b.WriteString("\nThe user is looking at project " + curSite + ".")
	}
	raw, err := brainJSON(ctx, b.String(), text, planSchemaFor(c), 500)
	if err != nil {
		return planDraft{}, err
	}
	var out struct {
		Steps []map[string]any `json:"steps"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return planDraft{}, fmt.Errorf("the offline brain's plan was not readable: %v", err)
	}
	var d planDraft
	for _, st := range out.Steps {
		tool, _ := st["tool"].(string)
		delete(st, "tool")
		for k, v := range st {
			s, ok := v.(string)
			if ok && k == "domain" { // small models add words around it
				s = reDomain.FindString(strings.ToLower(s))
			} else if ok {
				s = strings.Trim(s, " \t\"'`,.;:")
			}
			if !ok || s == "" {
				delete(st, k)
				continue
			}
			st[k] = s
		}
		// a small model may drop or garble names: take them from the request
		if pt := planToolByName(tool); pt != nil && contains(pt.Params, "domain") && sa(st, "domain") == "" {
			if dm := reDomain.FindString(strings.ToLower(text)); dm != "" {
				st["domain"] = dm
			}
		}
		if pt := planToolByName(tool); pt != nil && contains(pt.Params, "server") {
			if sv := sa(st, "server"); sv == "" || (sv != thisPCID && findServer(c, sv) == nil) {
				if t := serverInText(c, strings.ToLower(text)); t != "" {
					st["server"] = t
				} else if reThisPC.MatchString(strings.ToLower(text)) {
					st["server"] = thisPCID
				}
			}
		}
		if pt := planToolByName(tool); pt != nil && pt.Site && sa(st, "site") == "" {
			st["site"] = firstNonEmpty(curSite, d.site)
		}
		d.add(tool, st)
	}
	return d, nil
}

// ---------- validation ----------

var planOrder = map[string]int{}

func init() {
	for _, t := range planTools {
		planOrder[t.Name] = t.Order
	}
}

// validatePlan validates a draft: known tools, real projects and servers,
// prerequisites added, set-up steps in dependency order.
func validatePlan(c *Config, goal string, d planDraft, source string) (*Plan, error) {
	p := &Plan{ID: randHex(8), Goal: goal, Source: source, State: "ready", Created: time.Now(), Notes: d.notes, memo: map[string]string{}}
	newName := ""
	for _, s := range d.steps {
		if s.Tool == "create_project" {
			newName = sa(s.Args, "name")
		}
	}
	var keep []*PlanStep
	for _, s := range d.steps {
		t := planToolByName(s.Tool)
		if t == nil {
			return nil, fmt.Errorf("Jiv doesn't know how to %q", s.Tool)
		}
		args := map[string]any{}
		for _, k := range t.Params {
			if v := sa(s.Args, k); v != "" {
				args[k] = v
			}
		}
		if t.Site {
			sv := sa(args, "site")
			switch {
			case sv == "$new" || (newName != "" && (sv == "" || strings.EqualFold(sv, newName) || sv == slugify(newName))):
				if newName == "" {
					return nil, errors.New("which project? Name it in your request")
				}
				args["site"] = slugify(newName)
			case sv == "":
				return nil, errNeedSite
			default:
				st := resolveSiteArg(c, sv)
				if st == nil {
					return nil, fmt.Errorf("I couldn't find a project called %q", sv)
				}
				args["site"] = st.Slug
			}
		}
		switch s.Tool {
		case "create_project":
			if newName == "" || !validSlug(slugify(newName)) {
				return nil, errors.New("please give the new project a name (letters, numbers and dashes)")
			}
			if findSite(c, slugify(newName)) != nil {
				return nil, fmt.Errorf("a project called %q already exists", newName)
			}
		case "set_domain":
			nd, err := normalizeDomain(sa(args, "domain"))
			if err != nil {
				return nil, err
			}
			nd = strings.TrimPrefix(nd, "www.")
			for _, o := range c.Sites {
				if o.Domain == nd && o.Slug != sa(args, "site") {
					return nil, fmt.Errorf("%s is already used by project %q", nd, o.Name)
				}
			}
			args["domain"] = nd
		case "add_backup_server", "move_project":
			sv := sa(args, "server")
			if sv != thisPCID {
				pr := resolveServerArg(c, sv)
				if pr == nil {
					return nil, fmt.Errorf("I couldn't find a server called %q", sv)
				}
				args["server"] = pr.ID
				sv = pr.ID
			}
			if st := findSite(c, sa(args, "site")); st != nil && (st.Server == sv || contains(st.Servers, sv)) {
				if s.Tool == "move_project" {
					return nil, fmt.Errorf("%s is already on %s", st.Name, serverName(sv))
				}
				continue // already a backup there
			}
			if s.Tool == "move_project" && sv == thisPCID {
				return nil, errors.New("This PC can only be a backup server, not a project's main server")
			}
		case "free_disk", "disk_usage", "check_services", "restart_service":
			sv := sa(args, "server")
			if sv == "" || sv == "active" {
				sv = c.ActiveServer
			} else if pr := resolveServerArg(c, sv); pr != nil {
				sv = pr.ID
			}
			pr := findServer(c, sv)
			if pr == nil {
				return nil, errors.New("connect a server first")
			}
			if pr.Kind == "thispc" {
				return nil, errors.New("that works on servers, not on This PC")
			}
			args["server"] = sv
			if s.Tool == "restart_service" && sa(args, "service") == "" {
				args["service"] = "auto"
			}
		}
		keep = append(keep, &PlanStep{Tool: s.Tool, Args: args})
	}
	// prerequisites
	has := func(tool string) bool {
		for _, s := range keep {
			if s.Tool == tool {
				return true
			}
		}
		return false
	}
	for _, s := range keep {
		if s.Tool == "enable_https" {
			st := findSite(c, sa(s.Args, "site"))
			if !has("set_domain") && (st == nil || st.Domain == "") {
				return nil, errors.New("which domain? For example: put bakery on joesbakery.com with https")
			}
			if has("set_domain") && !has("dns_records") {
				keep = append(keep, &PlanStep{Tool: "dns_records", Args: map[string]any{"site": s.Args["site"]}})
			}
			if has("set_domain") && !has("wait_dns") {
				keep = append(keep, &PlanStep{Tool: "wait_dns", Args: map[string]any{"site": s.Args["site"]}})
			}
		}
		if s.Tool == "add_backup_server" && s.Args["server"] == thisPCID && !has("host_on_this_pc") && (c.ThisPC == nil || !c.ThisPC.Enabled) {
			keep = append(keep, &PlanStep{Tool: "host_on_this_pc", Args: map[string]any{}})
		}
		if s.Tool == "free_disk" && !has("disk_usage") { // always look before deleting
			keep = append(keep, &PlanStep{Tool: "disk_usage", Args: map[string]any{"server": s.Args["server"]}})
		}
		if s.Tool == "set_domain" && !has("enable_https") {
			keep = append(keep, &PlanStep{Tool: "dns_records", Args: map[string]any{"site": s.Args["site"]}}, &PlanStep{Tool: "wait_dns", Args: map[string]any{"site": s.Args["site"]}}, &PlanStep{Tool: "enable_https", Args: map[string]any{"site": s.Args["site"]}})
		}
	}
	// de-duplicate, then order: read-only checks first (as given), set-up steps by dependency
	seen := map[string]bool{}
	var uniq []*PlanStep
	for _, s := range keep {
		k := s.Tool + fmt.Sprint(s.Args)
		if !seen[k] {
			seen[k] = true
			uniq = append(uniq, s)
		}
	}
	sort.SliceStable(uniq, func(i, j int) bool { return planOrder[uniq[i].Tool] < planOrder[uniq[j].Tool] })
	if len(uniq) == 0 {
		return nil, errors.New("I couldn't turn that into steps")
	}
	for i, s := range uniq {
		t := planToolByName(s.Tool)
		s.N, s.State, s.Risky, s.Read = i+1, "pending", t.Risky, t.Read
		s.Label = t.label(s.Args)
		if t.detail != nil {
			s.Detail = t.detail(c, s.Args)
		}
		if sv := sa(s.Args, "site"); sv != "" {
			p.Site = sv
			if st := findSite(c, sv); st != nil && !onActive(c, st) && s.Tool != "move_project" && !s.Read {
				s.Detail = strings.TrimSpace(s.Detail + " " + st.Name + " is on " + serverLabel(c, st.Server) + "; Rover switches to it first.")
			}
		}
		if s.Tool == "dns_records" {
			s.Detail = dnsRecordsDetail(c, sa(s.Args, "site"), domainOf(uniq, c, sa(s.Args, "site")))
		}
	}
	p.Steps = uniq
	return p, nil
}

func domainOf(steps []*PlanStep, c *Config, slug string) string {
	for _, s := range steps {
		if s.Tool == "set_domain" && sa(s.Args, "site") == slug {
			return sa(s.Args, "domain")
		}
	}
	if st := findSite(c, slug); st != nil {
		return st.Domain
	}
	return ""
}

func dnsRecordsDetail(c *Config, slug, domain string) string {
	switch dnsMethod(c, domain) {
	case "cloudflare":
		return "Creates the records in your Cloudflare account automatically."
	case "godaddy", "porkbun":
		return "Creates the records with your registrar API key automatically."
	}
	return "Your registrar has no automatic way: Jiv pauses here and shows the two records to add, with copy buttons and a walkthrough, then checks by itself."
}

// planDNSHost is replaced in tests (the real one queries DNS/RDAP).
var planDNSHost = func(domain string) DNSHost { return detectDNSHost(domain) }

func dnsMethod(c *Config, domain string) string {
	if domain == "" {
		return "manual"
	}
	h := planDNSHost(domain)
	if c.CloudflareToken != "" && (h.ID == "cloudflare" || h.Auto) {
		return "cloudflare"
	}
	if c.Registrars != nil && c.Registrars[h.ID] != nil && (h.ID == "godaddy" || h.ID == "porkbun") {
		return h.ID
	}
	return "manual"
}

// ---------- store ----------

var (
	planMu sync.Mutex
	plans  = map[string]*Plan{}
)

func storePlan(p *Plan) {
	planMu.Lock()
	defer planMu.Unlock()
	for id, x := range plans {
		if time.Since(x.Created) > 12*time.Hour {
			delete(plans, id)
		}
	}
	plans[p.ID] = p
}

func getPlan(id string) *Plan {
	planMu.Lock()
	defer planMu.Unlock()
	return plans[id]
}

// snapshot copies a plan for JSON (the runner changes it concurrently).
func (p *Plan) snapshot() map[string]any {
	planMu.Lock()
	defer planMu.Unlock()
	bs, _ := json.Marshal(p)
	var out map[string]any
	_ = json.Unmarshal(bs, &out)
	return out
}

func planUpdate(fn func()) {
	planMu.Lock()
	defer planMu.Unlock()
	fn()
}

// makePlan is what the ask pipeline and POST /api/jiv/plan use.
func makePlan(ctx context.Context, c *Config, text, curSite string, useModel bool) (*Plan, planDraft, error) {
	d := planFromText(c, text, curSite)
	source := "templates"
	if len(d.steps) == 0 && useModel && brainAvailable(c) {
		md, err := planFromModel(ctx, c, text, curSite)
		if err == nil && len(md.steps) > 0 {
			d, source = md, "model"
		}
	}
	if d.needSite {
		return nil, d, errNeedSite
	}
	if len(d.steps) == 0 {
		return nil, d, errors.New("no steps")
	}
	p, err := validatePlan(c, text, d, source)
	if err != nil {
		return nil, d, err
	}
	storePlan(p)
	return p, d, nil
}

// ---------- HTTP ----------

func registerPlanRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/jiv/plan", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Q, Site string
			Model   bool
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		c := snapshotConfig()
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
		defer cancel()
		p, _, err := makePlan(ctx, &c, req.Q, req.Site, true)
		if errors.Is(err, errNeedSite) {
			var opts []map[string]string
			for _, s := range activeSites(&c) {
				opts = append(opts, map[string]string{"slug": s.Slug, "name": s.Name})
			}
			writeJSON(w, 200, map[string]any{"need_site": true, "sites": opts})
			return
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"plan": p.snapshot()})
	})
	mux.HandleFunc("GET /api/jiv/plans/{id}", func(w http.ResponseWriter, r *http.Request) {
		p := getPlan(r.PathValue("id"))
		if p == nil {
			writeJSON(w, 404, map[string]any{"error": "this plan expired; ask Jiv again"})
			return
		}
		writeJSON(w, 200, map[string]any{"plan": p.snapshot()})
	})
	act := func(name string, fn func(p *Plan, req planReq) (int, map[string]any)) {
		mux.HandleFunc("POST /api/jiv/plans/{id}/"+name, func(w http.ResponseWriter, r *http.Request) {
			var req planReq
			_ = readJSON(r, &req)
			p := getPlan(r.PathValue("id"))
			if p == nil {
				writeJSON(w, 404, map[string]any{"error": "this plan expired; ask Jiv again"})
				return
			}
			if !req.Confirmed && name != "cancel" {
				writeJSON(w, http.StatusPreconditionRequired, map[string]any{"error": "please confirm first", "need_confirm": true})
				return
			}
			code, out := fn(p, req)
			if out == nil {
				out = map[string]any{}
			}
			out["plan"] = p.snapshot()
			writeJSON(w, code, out)
		})
	}
	act("run", func(p *Plan, req planReq) (int, map[string]any) {
		ok := false
		planUpdate(func() {
			if p.State == "ready" {
				p.State, ok = "running", true
			}
		})
		if !ok {
			return 409, map[string]any{"error": "this plan already ran; ask Jiv again to repeat it"}
		}
		startPlan(p)
		return 200, nil
	})
	mux.HandleFunc("POST /api/jiv/plans/{id}/steps/{n}/confirm", func(w http.ResponseWriter, r *http.Request) {
		var req planReq
		_ = readJSON(r, &req)
		p := getPlan(r.PathValue("id"))
		n, _ := strconv.Atoi(r.PathValue("n"))
		if p == nil {
			writeJSON(w, 404, map[string]any{"error": "this plan expired; ask Jiv again"})
			return
		}
		ok := false
		planUpdate(func() {
			if n >= 1 && n <= len(p.Steps) {
				st := p.Steps[n-1]
				if req.Confirmed && st.State == "needs_confirm" && st.Token != "" && req.Token == st.Token {
					st.Token, st.State, ok = "", "confirmed", true
					p.State = "running"
				}
			}
		})
		if !ok {
			writeJSON(w, http.StatusPreconditionRequired, map[string]any{"error": "this step needs your confirm click", "need_confirm": true, "plan": p.snapshot()})
			return
		}
		startPlan(p)
		writeJSON(w, 200, map[string]any{"plan": p.snapshot()})
	})
	act("retry", func(p *Plan, req planReq) (int, map[string]any) {
		ok := false
		planUpdate(func() {
			for _, st := range p.Steps {
				if st.State == "failed" || st.State == "waiting" {
					st.State, st.Error, st.Fix, ok = "pending", "", nil, true
					break
				}
			}
			if ok {
				p.State = "running"
			}
		})
		if !ok {
			return 409, map[string]any{"error": "nothing to retry"}
		}
		startPlan(p)
		return 200, nil
	})
	act("skip", func(p *Plan, req planReq) (int, map[string]any) {
		ok := false
		planUpdate(func() {
			for _, st := range p.Steps {
				if st.State == "failed" || st.State == "needs_confirm" || st.State == "waiting" {
					st.State, st.Token, ok = "skipped", "", true
					break
				}
			}
			if ok {
				p.State = "running"
			}
		})
		if !ok {
			return 409, map[string]any{"error": "nothing to skip"}
		}
		startPlan(p)
		return 200, nil
	})
	act("cancel", func(p *Plan, req planReq) (int, map[string]any) {
		planUpdate(func() {
			if p.cancel != nil {
				p.cancel()
			}
			if p.State != "done" {
				p.State = "cancelled"
			}
			for _, st := range p.Steps {
				if st.State == "pending" || st.State == "needs_confirm" || st.State == "waiting" {
					st.State, st.Token = "skipped", ""
				}
			}
		})
		return 200, nil
	})
}

type planReq struct {
	Confirmed bool   `json:"confirmed"`
	Token     string `json:"token"`
}
