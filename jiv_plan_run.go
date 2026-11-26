package main

// Runs a Jiv plan step by step (see jiv_plan.go for the safety rules).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// hooks (tests replace them)
var (
	planPoll     = 10 * time.Second // DNS / HTTPS re-check interval
	planWaitDNS  = 45 * time.Minute // how long Jiv waits for DNS before asking
	planWaitTLS  = 4 * time.Minute
	planJobLimit = 30 * time.Minute
	planShell    = func(ctx context.Context, c *Config, serverID, script string) (string, error) {
		p := findServer(c, serverID)
		if p == nil {
			return "", fmt.Errorf("server %q is not in Rover any more", serverID)
		}
		r, err := runnerFor(c, p)
		if err != nil {
			return "", err
		}
		var out bytes.Buffer
		err = r.Run(ctx, script, &out)
		return out.String(), err
	}
	planHealth = func(slug string) *HealthResult { return runHealthFor([]string{slug})[slug] }
)

var errStepWaiting = errors.New("waiting for you")

func startPlan(p *Plan) {
	start := false
	planUpdate(func() {
		if !p.running && (p.State == "running") {
			p.running, start = true, true
			ctx, cancel := context.WithCancel(context.Background())
			p.ctx, p.cancel = ctx, cancel
		}
	})
	if start {
		go runPlan(p)
	}
}

func runPlan(p *Plan) {
	defer planUpdate(func() { p.running = false })
	for {
		var st *PlanStep
		done := true
		planUpdate(func() {
			if p.State == "cancelled" {
				return
			}
			for _, s := range p.Steps {
				switch s.State {
				case "pending", "confirmed":
					if st == nil {
						st = s
					}
					done = false
				case "failed", "needs_confirm", "waiting":
					done = false
				}
				if st != nil || s.State == "failed" || s.State == "needs_confirm" || s.State == "waiting" {
					break
				}
			}
			if st == nil {
				switch {
				case done:
					p.State = "done"
				default:
					for _, s := range p.Steps {
						if s.State == "failed" {
							p.State = "failed"
						}
					}
				}
				return
			}
			if (st.Risky || (p.Auto && !st.Read)) && st.State == "pending" {
				st.State, st.Token = "needs_confirm", randHex(12)
				p.State = "waiting"
				st = nil
				return
			}
			st.State, st.Started = "running", time.Now().Format(time.RFC3339)
		})
		if st == nil {
			return
		}
		res, err := execStep(p.ctx, p, st)
		planUpdate(func() {
			if st.State != "running" && st.State != "waiting" { // skipped or cancelled meanwhile
				return
			}
			if err != nil {
				if p.ctx.Err() != nil {
					st.State, st.Error = "skipped", "cancelled"
					return
				}
				st.State, st.Error = "failed", err.Error()
				if st.Fix == nil {
					st.Fix = fixFor(p, st, err)
				}
				p.State = "failed"
				return
			}
			st.State, st.Result = "done", res
		})
		if p.State == "failed" || p.State == "cancelled" {
			return
		}
	}
}

// fixFor explains a failed step in plain words and offers the next move.
func fixFor(p *Plan, st *PlanStep, err error) *PlanFix {
	title, text, kb := explainError(err.Error())
	f := &PlanFix{Title: title, Text: text, KB: kb}
	site := sa(st.Args, "site")
	low := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, errNotConnected) || strings.Contains(low, "not connected"):
		f.Card, _ = proposeAction(ToolCall{Name: "diagnose", Args: map[string]any{"site": site}})
		f.Guide = "guide-setup-server"
	case strings.Contains(low, "dns not ready"):
		f.Text = "The domain does not point to your server yet. Jiv can wait for it: press Retry in a few minutes."
		f.Guide = "guide-domain-godaddy"
	case strings.Contains(low, "no space left") || strings.Contains(low, "disk is full") || strings.Contains(low, "disk full"):
		f.Title, f.Text = "The server's disk is full", "Free some space, then retry. Ask Jiv: free disk space on my server."
	case strings.Contains(low, "nginx rejected"):
		f.Text = "The site's _redirects or _headers file has a line nginx doesn't accept. Fix that line (the log names it), or roll back."
		f.KB = firstNonEmpty(kb, "redirects-headers")
	case st.Tool == "add_backup_server" && strings.Contains(low, "tunnel"):
		f.Guide = "guide-this-pc"
	}
	if site != "" && f.Card == nil && (st.Tool == "deploy_folder" || st.Tool == "retry_deploy" || st.Tool == "fix_web_config") {
		f.Card, _ = proposeAction(ToolCall{Name: "rollback_site", Args: map[string]any{"site": site}})
	}
	return f
}

func (st *PlanStep) logf(format string, a ...any) {
	planUpdate(func() {
		st.Log = append(st.Log, fmt.Sprintf(format, a...))
		if len(st.Log) > 40 {
			st.Log = st.Log[len(st.Log)-40:]
		}
	})
}

// awaitJob follows an app job, copying its log into the step.
func awaitJob(ctx context.Context, st *PlanStep, out map[string]any) error {
	id, _ := out["job"].(string)
	if id == "" {
		return nil
	}
	j := getJob(id)
	if j == nil {
		return errors.New("the task disappeared")
	}
	seen := 0
	deadline := time.Now().Add(planJobLimit)
	for {
		j.mu.Lock()
		lines := append([]string(nil), j.lines...)
		done, jerr := j.done, j.err
		j.mu.Unlock()
		for ; seen < len(lines); seen++ {
			st.logf("%s", lines[seen])
		}
		if done {
			if jerr != "" {
				return errors.New(jerr)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("this is taking much longer than usual; check the Activity page")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func callErr(code int, out map[string]any) error {
	if code == 200 {
		return nil
	}
	if e, _ := out["error"].(string); e != "" {
		return errors.New(e)
	}
	return fmt.Errorf("Rover answered %d", code)
}

// jivTool runs one of Jiv's existing actions (the plan was confirmed).
func jivTool(name string, args map[string]any) (map[string]any, error) {
	card, err := proposeAction(ToolCall{Name: name, Args: args})
	if err != nil {
		return nil, err
	}
	code, out := runAction(card.ID, true)
	return out, callErr(code, out)
}

func (p *Plan) slug(st *PlanStep) string { return sa(st.Args, "site") }

// ensureActive switches to the project's server when a step needs it.
func ensureActive(st *PlanStep, slug string) error {
	c := snapshotConfig()
	s := findSite(&c, slug)
	if s == nil {
		return fmt.Errorf("project %q is gone", slug)
	}
	if onActive(&c, s) || s.Server == "" {
		return nil
	}
	st.logf("Switching to %s first.", serverLabel(&c, s.Server))
	return activateServer(s.Server)
}

func execStep(ctx context.Context, p *Plan, st *PlanStep) (string, error) {
	c := snapshotConfig()
	slug := p.slug(st)
	if t := planToolByName(st.Tool); t != nil && t.Site && !t.Read && st.Tool != "move_project" {
		if err := ensureActive(st, slug); err != nil {
			return "", err
		}
		c = snapshotConfig()
	}
	s := findSite(&c, slug)
	switch st.Tool {
	case "create_project":
		code, out := innerCall("POST", "/api/sites", map[string]any{"name": sa(st.Args, "name")})
		if err := callErr(code, out); err != nil {
			return "", err
		}
		if err := awaitJob(ctx, st, out); err != nil {
			return "", err
		}
		sl, _ := out["slug"].(string)
		p.newSite = sl
		c = snapshotConfig()
		if s := findSite(&c, sl); s != nil && s.InstantURL != "" {
			return "Created. Free address: " + s.InstantURL, nil
		}
		return "Created.", nil
	case "deploy_folder":
		out, err := jivTool("deploy_folder", map[string]any{"site": slug})
		if err != nil {
			return "", err
		}
		if err := awaitJob(ctx, st, out); err != nil {
			return "", err
		}
		return "Published.", nil
	case "retry_deploy":
		code, out := innerCall("POST", "/api/sites/"+slug+"/redeploy-last", map[string]any{})
		if err := callErr(code, out); err != nil {
			return "", err
		}
		if err := awaitJob(ctx, st, out); err != nil {
			return "", err
		}
		return "Published again.", nil
	case "host_on_this_pc":
		x := ThisPCConfig{Port: 8080}
		if c.ThisPC != nil {
			x = *c.ThisPC
		}
		x.Enabled, x.PreventSleep = true, true
		code, out := innerCall("POST", "/api/thispc", x)
		if err := callErr(code, out); err != nil {
			return "", err
		}
		return "This PC now hosts sites (port " + strconv.Itoa(firstPort(x.Port)) + ").", nil
	case "add_backup_server":
		code, out := innerCall("POST", "/api/sites/"+slug+"/servers", map[string]any{"server": sa(st.Args, "server"), "mode": "backup"})
		if err := callErr(code, out); err != nil {
			return "", err
		}
		if err := awaitJob(ctx, st, out); err != nil {
			return "", err
		}
		return serverName(sa(st.Args, "server")) + " now keeps a copy and takes over if the main server goes down.", nil
	case "set_domain":
		d := sa(st.Args, "domain")
		www := isApex(d)
		code, out := innerCall("POST", "/api/sites/"+slug+"/domain", map[string]any{"domain": d, "www": www})
		if err := callErr(code, out); err != nil {
			return "", err
		}
		if www {
			return "Saved " + d + " (and www." + d + ").", nil
		}
		return "Saved " + d + ".", nil
	case "dns_records":
		return stepDNS(ctx, p, st, &c, s)
	case "wait_dns":
		return stepWaitDNS(ctx, st, s, planWaitDNS)
	case "enable_https":
		if s == nil || s.Domain == "" {
			return "", errors.New("no domain saved for this project")
		}
		if activeLite(&c) || s.TunnelID != "" {
			code, out := innerCall("POST", "/api/sites/"+slug+"/domain/finish", nil)
			if err := callErr(code, out); err != nil {
				return "", err
			}
		} else {
			code, out := innerCall("POST", "/api/sites/"+slug+"/domain/apply", map[string]any{})
			if err := callErr(code, out); err != nil {
				return "", err
			}
		}
		st.logf("Waiting for the free HTTPS certificate (usually under a minute)…")
		deadline := time.Now().Add(planWaitTLS)
		for {
			ok, msg := guideHTTPS(slug)
			if ok {
				_ = withConfig(func(c *Config) error {
					if x := findSite(c, slug); x != nil {
						x.HTTPSActive = true
					}
					return nil
				})
				return "https://" + s.Domain + " is live with a padlock.", nil
			}
			if time.Now().After(deadline) {
				return "", fmt.Errorf("HTTPS is not working yet (%s). Certificates can take a few minutes; press Retry", msg)
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(planPoll):
			}
		}
	case "rollback":
		out, err := jivTool("rollback_site", map[string]any{"site": slug})
		if err != nil {
			return "", err
		}
		return fmt.Sprint(out["message"]), nil
	case "restart_site":
		out, err := jivTool("redeploy_site", map[string]any{"site": slug})
		if err != nil {
			return "", err
		}
		return fmt.Sprint(out["message"]), nil
	case "backup_now":
		out, err := jivTool("backup_now", nil)
		if err != nil {
			return "", err
		}
		if err := awaitJob(ctx, st, out); err != nil {
			return "", err
		}
		return "Backup finished.", nil
	case "diagnose", "check_dns", "explain_logs":
		out, err := jivTool(st.Tool, map[string]any{"site": slug})
		if err != nil {
			return "", err
		}
		if d, ok := out["diagnosis"].(Diagnosis); ok {
			planUpdate(func() { st.Diag = &d })
		}
		return fmt.Sprint(out["message"]), nil
	case "why_deploy_failed":
		return stepWhyFailed(p, st, &c, s)
	case "fix_web_config":
		code, out := innerCall("POST", "/api/sites/"+slug+"/apply", nil)
		if err := callErr(code, out); err != nil {
			return "", err
		}
		if err := awaitJob(ctx, st, out); err != nil {
			return "", err
		}
		h := planHealth(slug)
		if h != nil && !h.OK {
			t, _, _ := explainError(h.Error)
			return "", fmt.Errorf("the settings were rebuilt but the site still has a problem: %s", t)
		}
		return "The web server settings were rebuilt and tested; the site answers.", nil
	case "disk_usage":
		out, err := planShell(ctx, &c, sa(st.Args, "server"), diskUsageScript)
		if err != nil {
			return "", err
		}
		return summarizeDisk(out), nil
	case "free_disk":
		out, err := planShell(ctx, &c, sa(st.Args, "server"), freeDiskScript)
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			if l != "" && !strings.HasPrefix(l, "HP_") {
				st.logf("%s", l)
			}
		}
		if err != nil {
			return "", err
		}
		return summarizeFreed(out), nil
	case "check_services":
		out, err := planShell(ctx, &c, sa(st.Args, "server"), checkServicesScript)
		if err != nil {
			return "", err
		}
		bad := parseStuck(out)
		planUpdate(func() { p.memo["stuck:"+sa(st.Args, "server")] = strings.Join(bad, "\n") })
		if len(bad) == 0 {
			return "Nothing looks stuck: all services are running.", nil
		}
		return "Not running properly: " + strings.Join(friendlyStuck(bad), ", ") + ".", nil
	case "restart_service":
		return stepRestart(ctx, p, st, &c)
	case "renew_ssl":
		return stepRenewSSL(ctx, st, &c, s)
	case "move_project":
		code, out := innerCall("POST", "/api/sites/"+slug+"/server", map[string]any{"server": sa(st.Args, "server")})
		if err := callErr(code, out); err != nil {
			return "", err
		}
		if err := awaitJob(ctx, st, out); err != nil {
			return "", err
		}
		res := s.Name + " now runs on " + serverName(sa(st.Args, "server")) + "."
		if s.Domain != "" {
			res += " Point " + s.Domain + " to the new server's address (Jiv's domain walkthrough shows how), then turn off the old copy."
		}
		return res, nil
	}
	return "", fmt.Errorf("Jiv can't run %q", st.Tool)
}

func firstPort(p int) int {
	if p == 0 {
		return 8080
	}
	return p
}

func stepDNS(ctx context.Context, p *Plan, st *PlanStep, c *Config, s *Site) (string, error) {
	if s == nil || s.Domain == "" {
		return "", errors.New("save the domain first")
	}
	switch dnsMethod(c, s.Domain) {
	case "cloudflare":
		code, out := innerCall("POST", "/api/sites/"+s.Slug+"/auto-dns", map[string]any{"domain": s.Domain, "www": s.IncludeWWW})
		if code == 409 || out["conflicts"] != nil {
			return "", errors.New("the domain already has records pointing somewhere else. Open the Domain tab to replace them (Rover shows exactly what changes)")
		}
		if err := callErr(code, out); err != nil {
			return "", err
		}
		return "Records created in Cloudflare.", nil
	case "godaddy", "porkbun":
		h := planDNSHost(s.Domain)
		code, out := innerCall("POST", "/api/sites/"+s.Slug+"/registrar-dns", map[string]any{"provider": h.ID, "domain": s.Domain, "www": s.IncludeWWW})
		if code == 409 || out["conflicts"] != nil {
			return "", errors.New("the domain already has records pointing somewhere else. Open the Domain tab to replace them")
		}
		if err := callErr(code, out); err != nil {
			return "", err
		}
		return "Records created at " + h.Name + ".", nil
	}
	// manual: show the records and wait until they work
	ip := guideVars(c, s)["ip"]
	h := planDNSHost(s.Domain)
	var recs []string
	for _, r := range requiredRecords(s.Domain, s.IncludeWWW, ip) {
		recs = append(recs, fmt.Sprintf("%s record  name %s  →  %s", r.Type, r.Name, r.Value))
	}
	guide := "guide-domain-godaddy"
	if h.ID == "cloudflare" {
		guide = "guide-domain-cloudflare"
	}
	planUpdate(func() {
		st.State, st.Guide = "waiting", guide
		p.State = "waiting"
		st.Detail = "Add these at " + firstNonEmpty(h.Name, "your domain company") + ": " + strings.Join(recs, "; ") + ". Jiv checks every few seconds and continues by itself."
	})
	res, err := stepWaitDNS(ctx, st, s, planWaitDNS)
	planUpdate(func() {
		if st.State == "waiting" {
			st.State = "running"
			p.State = "running"
		}
	})
	return res, err
}

func stepWaitDNS(ctx context.Context, st *PlanStep, s *Site, limit time.Duration) (string, error) {
	if s == nil || s.Domain == "" {
		return "", errors.New("save the domain first")
	}
	c := snapshotConfig()
	ip := guideVars(&c, s)["ip"]
	hosts := []string{s.Domain}
	if s.IncludeWWW {
		hosts = append(hosts, "www."+s.Domain)
	}
	deadline := time.Now().Add(limit)
	last := ""
	for {
		all, msg := true, ""
		for _, h := range hosts {
			if ok, m := guideDNSOK(h, ip); !ok {
				all, msg = false, h+": "+m
				break
			}
		}
		if all {
			return s.Domain + " points to your server.", nil
		}
		if msg != last {
			st.logf("%s", msg)
			last = msg
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("DNS not ready: %s", msg)
		}
		skipped := false
		planUpdate(func() { skipped = st.State == "skipped" })
		if skipped {
			return "", errors.New("skipped")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(planPoll):
		}
	}
}

func stepWhyFailed(p *Plan, st *PlanStep, c *Config, s *Site) (string, error) {
	if s == nil {
		return "", errors.New("unknown project")
	}
	dir, _, err := projectDir(s.Slug)
	if err != nil {
		return "", err
	}
	ix, _ := readIndex(dir)
	var bad *DeployRecord
	if ix != nil {
		for _, d := range ix.Deploys {
			if d.Status == "failed" || d.Status == "partial" {
				bad = d
				break
			}
			if d.Status == "published" || d.Status == "live" {
				break // the newest deploy worked
			}
		}
	}
	if bad == nil {
		return "The last deploy worked; nothing failed recently. If the site looks wrong, ask Jiv to check it.", nil
	}
	title, text, kb := explainError(bad.Error)
	d := Diagnosis{Site: s.Slug, Name: s.Name, Summary: title, Findings: []Finding{{Level: "fail", Title: title, Text: text, KB: kb}}}
	low := strings.ToLower(bad.Error)
	var add []*PlanStep
	switch {
	case strings.Contains(low, "no space left") || strings.Contains(low, "disk full"):
		srv := firstNonEmpty(s.Server, c.ActiveServer)
		add = append(add, &PlanStep{Tool: "free_disk", Args: map[string]any{"server": srv}}, &PlanStep{Tool: "retry_deploy", Args: map[string]any{"site": s.Slug}})
	case strings.Contains(low, "nginx rejected") || strings.Contains(low, "redirect") || strings.Contains(low, "_headers"):
		d.Findings[0].Text += " Fix that line in the project's files, then publish again. The live site was not changed."
		d.Findings[0].KB = firstNonEmpty(kb, "redirects-headers")
	case strings.Contains(low, "index.html") || strings.Contains(low, "build"):
		d.Findings[0].Href = "#/project/" + s.Slug + "/deploys"
	default: // network, timeouts, a busy server: publishing again usually works
		add = append(add, &PlanStep{Tool: "retry_deploy", Args: map[string]any{"site": s.Slug}})
	}
	planUpdate(func() {
		st.Diag = &d
		// insert the fix steps right after this one
		idx := st.N
		for _, a := range add {
			t := planToolByName(a.Tool)
			a.State, a.Risky, a.Added, a.Label = "pending", t.Risky, true, t.label(a.Args)
			if t.detail != nil {
				a.Detail = t.detail(c, a.Args)
			}
		}
		rest := append(add, p.Steps[idx:]...)
		p.Steps = append(p.Steps[:idx:idx], rest...)
		for i, x := range p.Steps {
			x.N = i + 1
		}
	})
	res := "Deploy " + bad.ID + " failed: " + title + ". " + text
	if len(add) > 0 {
		res += " Jiv added the fix to the plan."
	}
	return res, nil
}

// ---------- server scripts ----------

const diskUsageScript = `df -Pk / | awk 'NR==2{print "HP_DISK "$2" "$3" "$4}'
if command -v docker >/dev/null 2>&1; then
  docker system df --format '{{.Type}}\t{{.Size}}\t{{.Reclaimable}}' 2>/dev/null | sed 's/^/HP_DOCKER\t/'
fi
du -sk /var/log 2>/dev/null | awk '{print "HP_LOGS "$1}'
du -sk /var/cache/apt 2>/dev/null | awk '{print "HP_APT "$1}'
du -sk /data/hostpanel 2>/dev/null | awk '{print "HP_SITES "$1}'
`

// freeDiskScript never touches running containers, volumes or site files.
// HP_DRY=1 prints what it would do instead.
const freeDiskScript = `set -u
run() { if [ "${HP_DRY:-0}" = 1 ]; then echo "would run: $*"; else "$@"; fi; }
df -Pk / | awk 'NR==2{print "HP_BEFORE "$4}'
if command -v docker >/dev/null 2>&1; then
  echo "Removing unused Docker images older than 3 days…"
  run docker image prune -af --filter until=72h 2>&1 | tail -n 1
  echo "Removing old Docker build cache…"
  run docker builder prune -af --filter until=72h 2>&1 | tail -n 1
fi
if command -v journalctl >/dev/null 2>&1; then
  echo "Shrinking system logs to 200 MB…"
  run journalctl --vacuum-size=200M 2>&1 | tail -n 1
fi
if command -v apt-get >/dev/null 2>&1; then
  echo "Clearing downloaded update packages…"
  run apt-get clean
fi
find /var/log -name '*.gz' -mtime +14 -type f 2>/dev/null | while read -r f; do run rm -f "$f"; done
df -Pk / | awk 'NR==2{print "HP_AFTER "$4" "$2}'
`

const checkServicesScript = `if command -v docker >/dev/null 2>&1; then
  docker ps -a --format '{{.Names}}|{{.Status}}' 2>/dev/null | while IFS='|' read -r n s; do
    case "$s" in
      *unhealthy*|Restarting*) echo "HP_BAD docker $n";;
      Exited*|Dead*) case "$n" in coolify|coolify-proxy|coolify-db|coolify-redis|coolify-realtime|hp-*) echo "HP_BAD docker $n";; esac;;
    esac
  done
fi
for u in nginx docker cloudflared; do
  if systemctl is-enabled "$u" >/dev/null 2>&1; then
    systemctl is-active --quiet "$u" || echo "HP_BAD unit $u"
  fi
done
systemctl list-units --failed --no-legend --plain 2>/dev/null | awk '{print $1}' | grep -E '^(dilreet|cloudflared|nginx|docker)' | sed 's/\.service$//' | sed 's/^/HP_BAD unit /'
true
`

var reSafeName = regexp.MustCompile(`^[A-Za-z0-9_.@-]{1,80}$`)

func parseStuck(out string) []string {
	seen := map[string]bool{}
	var bad []string
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 3 && f[0] == "HP_BAD" && (f[1] == "docker" || f[1] == "unit") && reSafeName.MatchString(f[2]) && !seen[f[1]+" "+f[2]] {
			seen[f[1]+" "+f[2]] = true
			bad = append(bad, f[1]+" "+f[2])
		}
	}
	return bad
}

func friendlyStuck(bad []string) []string {
	var out []string
	for _, b := range bad {
		_, n, _ := strings.Cut(b, " ")
		switch {
		case n == "coolify-proxy":
			out = append(out, "the web proxy (HTTPS)")
		case strings.HasPrefix(n, "coolify"):
			out = append(out, "the hosting engine ("+n+")")
		case strings.HasPrefix(n, "cloudflared") || strings.HasPrefix(n, "dilreet-tunnel"):
			out = append(out, "the Cloudflare tunnel")
		default:
			out = append(out, n)
		}
	}
	return out
}

func restartScript(items []string) string {
	var b strings.Builder
	b.WriteString("set -u\n")
	for _, it := range items {
		kind, n, _ := strings.Cut(it, " ")
		if !reSafeName.MatchString(n) {
			continue
		}
		switch kind {
		case "docker":
			fmt.Fprintf(&b, "echo 'Restarting %[1]s…'; docker restart %[1]s >/dev/null && sleep 3 && [ \"$(docker inspect -f '{{.State.Running}}' %[1]s)\" = true ] && echo 'HP_OK docker %[1]s' || echo 'HP_STILL docker %[1]s'\n", n)
		case "unit":
			pre := ""
			if n == "nginx" {
				pre = "nginx -t 2>&1 | tail -n 2; "
			}
			fmt.Fprintf(&b, "echo 'Restarting %[1]s…'; %[2]ssystemctl restart %[1]s && sleep 2 && systemctl is-active --quiet %[1]s && echo 'HP_OK unit %[1]s' || echo 'HP_STILL unit %[1]s'\n", n, pre)
		}
	}
	return b.String()
}

func stepRestart(ctx context.Context, p *Plan, st *PlanStep, c *Config) (string, error) {
	srv, svc := sa(st.Args, "server"), sa(st.Args, "service")
	var items []string
	switch svc {
	case "auto", "":
		var memo string
		planUpdate(func() { memo = p.memo["stuck:"+srv] })
		if strings.TrimSpace(memo) == "" {
			out, err := planShell(ctx, c, srv, checkServicesScript)
			if err != nil {
				return "", err
			}
			items = parseStuck(out)
		} else {
			items = strings.Split(memo, "\n")
		}
		if len(items) == 0 {
			return "Nothing was stuck, so nothing was restarted.", nil
		}
	case "proxy":
		if pr := findServer(c, srv); pr != nil && pr.Engine == "lite" {
			items = []string{"unit nginx"}
		} else {
			items = []string{"docker coolify-proxy"}
		}
	case "nginx":
		items = []string{"unit nginx"}
	case "coolify":
		items = []string{"docker coolify"}
	case "docker":
		items = []string{"unit docker"}
	case "tunnel":
		items = []string{"unit cloudflared"}
	default:
		return "", fmt.Errorf("unknown service %q", svc)
	}
	out, err := planShell(ctx, c, srv, restartScript(items))
	if err != nil {
		return "", err
	}
	var ok, still []string
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 3 && f[0] == "HP_OK" {
			ok = append(ok, f[1]+" "+f[2])
		} else if len(f) == 3 && f[0] == "HP_STILL" {
			still = append(still, f[1]+" "+f[2])
		} else if strings.TrimSpace(l) != "" {
			st.logf("%s", l)
		}
	}
	if len(still) > 0 {
		return "", fmt.Errorf("%s did not come back after the restart; open the server's log (Settings > Server) or ask Jiv to explain it", strings.Join(friendlyStuck(still), ", "))
	}
	return "Restarted " + strings.Join(friendlyStuck(ok), ", ") + "; running again.", nil
}

func stepRenewSSL(ctx context.Context, st *PlanStep, c *Config, s *Site) (string, error) {
	if s == nil || s.Domain == "" {
		return "", errors.New("this project has no domain, so its certificate is the free address's one (renewed automatically)")
	}
	h := planHealth(s.Slug)
	if h != nil && h.OK && h.CertDays != nil && *h.CertDays >= 21 {
		return fmt.Sprintf("The certificate is valid for %d more days and renews by itself; nothing to do.", *h.CertDays), nil
	}
	pr := findServer(c, firstNonEmpty(s.Server, c.ActiveServer))
	if s.TunnelID != "" || (pr != nil && pr.Engine == "lite") {
		return "", errors.New("this site's HTTPS comes from Cloudflare (tunnel). In Cloudflare open SSL/TLS > Edge Certificates and check the certificate is Active; Rover can't renew it from here")
	}
	if pr == nil {
		return "", errNotConnected
	}
	st.logf("Restarting the web proxy so it asks Let's Encrypt for a fresh certificate…")
	out, err := planShell(ctx, c, pr.ID, restartScript([]string{"docker coolify-proxy"}))
	if err != nil {
		return "", err
	}
	if strings.Contains(out, "HP_STILL") {
		return "", errors.New("the web proxy did not come back after the restart")
	}
	deadline := time.Now().Add(planWaitTLS)
	for {
		h = planHealth(s.Slug)
		if h != nil && h.OK && (h.CertDays == nil || *h.CertDays >= 21) {
			if h.CertDays != nil {
				return fmt.Sprintf("Renewed: the certificate is now valid for %d days.", *h.CertDays), nil
			}
			return "HTTPS works again.", nil
		}
		if time.Now().After(deadline) {
			msg := "no answer"
			if h != nil && h.Error != "" {
				msg, _, _ = explainError(h.Error)
			}
			return "", fmt.Errorf("the certificate was not renewed yet (%s). Let's Encrypt needs the domain to point to this server on port 80; ask Jiv to check the domain", msg)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(planPoll):
		}
	}
}

func kb2human(kb int64) string {
	switch {
	case kb >= 1<<20:
		return fmt.Sprintf("%.1f GB", float64(kb)/float64(1<<20))
	case kb >= 1<<10:
		return fmt.Sprintf("%d MB", kb>>10)
	}
	return fmt.Sprintf("%d KB", kb)
}

func summarizeDisk(out string) string {
	var parts []string
	var dockerRecl []string
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 0 {
			continue
		}
		num := func(i int) int64 { n, _ := strconv.ParseInt(f[i], 10, 64); return n }
		switch f[0] {
		case "HP_DISK":
			if len(f) >= 4 && num(1) > 0 {
				parts = append(parts, fmt.Sprintf("Disk: %s of %s used (%d%%), %s free", kb2human(num(2)), kb2human(num(1)), num(2)*100/num(1), kb2human(num(3))))
			}
		case "HP_DOCKER":
			t := strings.Split(l, "\t")
			if len(t) >= 4 && !strings.HasPrefix(t[3], "0B") {
				dockerRecl = append(dockerRecl, strings.ToLower(t[1])+" "+strings.Fields(t[3])[0])
			}
		case "HP_LOGS":
			if len(f) >= 2 {
				parts = append(parts, "system logs "+kb2human(num(1)))
			}
		case "HP_APT":
			if len(f) >= 2 {
				parts = append(parts, "package downloads "+kb2human(num(1)))
			}
		case "HP_SITES":
			if len(f) >= 2 {
				parts = append(parts, "your sites "+kb2human(num(1)))
			}
		}
	}
	if len(dockerRecl) > 0 {
		parts = append(parts, "Docker space that can be freed: "+strings.Join(dockerRecl, ", "))
	}
	if len(parts) == 0 {
		return "Couldn't read the disk usage."
	}
	return strings.Join(parts, "; ") + "."
}

func summarizeFreed(out string) string {
	var before, after, total int64
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 && f[0] == "HP_BEFORE" {
			before, _ = strconv.ParseInt(f[1], 10, 64)
		}
		if len(f) >= 3 && f[0] == "HP_AFTER" {
			after, _ = strconv.ParseInt(f[1], 10, 64)
			total, _ = strconv.ParseInt(f[2], 10, 64)
		}
	}
	if after == 0 {
		return "Cleanup finished."
	}
	freed := after - before
	if freed < 0 {
		freed = 0
	}
	s := fmt.Sprintf("Freed %s. Free space now: %s", kb2human(freed), kb2human(after))
	if total > 0 {
		s += fmt.Sprintf(" of %s", kb2human(total))
	}
	return s + "."
}
