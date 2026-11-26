package main

// Jiv's diagnosis: reads live app state (server connection, the hosting
// engine, deploy history, health checks, DNS) and explains problems in
// plain words with a fix button. Read-only: nothing here changes anything.

import (
	"fmt"
	"regexp"
	"strings"
)

type Finding struct {
	Level  string    `json:"level"` // ok | info | warn | fail
	Title  string    `json:"title"`
	Text   string    `json:"text"`
	Fix    *ToolCall `json:"fix,omitempty"`  // suggested action (goes through the normal confirm rules)
	Href   string    `json:"href,omitempty"` // or a screen to open
	FixLbl string    `json:"fix_label,omitempty"`
	KB     string    `json:"kb,omitempty"` // article id for "learn more"
}

type Diagnosis struct {
	Site     string    `json:"site,omitempty"`
	Name     string    `json:"name,omitempty"`
	Summary  string    `json:"summary"`
	Findings []Finding `json:"findings"`
}

// explainError turns a raw error into a cause and a fix.
type errPattern struct {
	re          *regexp.Regexp
	title, text string
	kb          string
}

var errPatterns = []errPattern{
	// 0.8: more specific messages first
	{regexp.MustCompile(`(?i)too many redirects|redirect loop|redirected you too many times`), "The site redirects in a loop", "Usually Cloudflare's SSL mode is Flexible (choose Full (strict)), or a _redirects rule points a page to itself. Ask Jiv: fix the redirects.", "error-redirect-loop"},
	{regexp.MustCompile(`(?i)address already in use|only one usage of each socket address`), "That port is already used by another program", "Choose another port (for example 8081) on the This PC page and save.", "error-port-in-use"},
	{regexp.MustCompile(`(?i)sudo: (a )?password is required|not in the sudoers`), "The server user can't run admin commands", "Connect as root, or as the default ubuntu user (password-less sudo).", "error-sudo"},
	{regexp.MustCompile(`(?i)certificate has expired|cert_date_invalid|expired certificate`), "The HTTPS certificate has expired", "Ask Jiv to renew the SSL: it checks the certificate and restarts the web proxy so it renews.", "error-cert-expired"},
	{regexp.MustCompile(`(?i)cloudflare.*\b52[1256]\b|\berror 52[1256]\b`), "Cloudflare can't reach your server properly", "Check the server is up with ports 80/443 open; for 525/526 set SSL/TLS to Full (strict) after the certificate is issued.", "error-cloudflare-526"},
	{regexp.MustCompile(`(?i)not a valid zip|zip: not a valid`), "The zip file can't be opened", "Make the zip again with Windows (Send to > Compressed folder) or drop the folder itself.", "error-zip-invalid"},
	{regexp.MustCompile(`(?i)checksum mismatch|is damaged`), "A file is damaged", "Copy or download it again; for split packs keep all parts together.", "error-pack-checksum"},
	{regexp.MustCompile(`(?i)vulkan|ggml_vulkan|failed to allocate.*(vram|device)`), "The graphics card couldn't load the model", "Jiv uses the processor instead. Update the graphics driver and press Restart in Settings > Jiv to try the card again.", "gpu-brain"},
	{regexp.MustCompile(`(?i)no index\.html|index\.html (is )?missing|no home ?page|without an index`), "No index.html in the top folder", "Visitors need an index.html at the top of your upload. Zip the contents of your site folder (not the folder itself), or rename your home page to index.html.", "error-index-missing"},
	{regexp.MustCompile(`(?i)must be built|package\.json|needs? (a )?build`), "The project must be built first", "This is source code (React/Vue/Vite...). Click Build it for me (needs Node.js), or build it yourself and drop the dist/build folder.", "build-node"},
	{regexp.MustCompile(`(?i)npm.*(not found|not recognized)|node(\.exe)? (is )?not (found|recognized)|executable file not found.*(npm|node)`), "Node.js is not installed", "Install Node.js LTS from nodejs.org, restart Rover and build again.", "error-node-missing"},
	{regexp.MustCompile(`(?i)nginx rejected`), "A redirect/header rule could not be used", "The live site was not changed. App Testing > Check report shows which _redirects/_headers line to fix.", "nginx-rejected"},
	{regexp.MustCompile(`(?i)unable to authenticate|permission denied \(publickey|auth(entication)? failed|handshake failed: ssh`), "The server refused the login", "The password or key was not accepted. Type the root (or ubuntu) password once in Settings > Server, or use the key file from your provider.", "ssh-failed"},
	{regexp.MustCompile(`(?i)host key (changed|mismatch)|knownhosts: key mismatch`), "The server's identity changed", "Normal after a reinstall: click Forget host key in Settings > Server. If you changed nothing, stop and ask your provider.", "host-key"},
	{regexp.MustCompile(`(?i)not connected|connection refused|no route to host|i/o timeout|dial tcp|network is unreachable|connection reset`), "Rover can't reach the server", "Check the server is running in your provider panel and that port 22 is open, then click Connect in Settings > Server.", "ssh-failed"},
	{regexp.MustCompile(`(?i)\b401\b|unauthenticated|invalid token|token.*(expired|invalid)`), "The hosting engine rejected Rover's token", "Run Settings > Server > Set up automatically again (it makes a new token), or paste a new Coolify token.", "coolify-token"},
	{regexp.MustCompile(`(?i)\b403\b|forbidden|permission`), "Permission denied", "The token or account is missing a permission. For Coolify the token needs deploy (or root) rights.", "coolify-token"},
	{regexp.MustCompile(`(?i)no space left|disk (is )?full|enospc`), "The server disk is full", "Ask Jiv: free disk space on my server (it removes unused Docker images and old logs after your confirm), or make the disk bigger at your provider.", "server-full"},
	{regexp.MustCompile(`(?i)out of memory|oom|cannot allocate memory|killed`), "The server ran out of memory", "Give the server more RAM at your provider (4 GB+ for many sites), or restart it.", "server-full"},
	{regexp.MustCompile(`(?i)too large|too big|exceeds`), "The upload is too large", "Remove videos, backups and node_modules from the folder, then deploy again.", "error-too-large"},
	{regexp.MustCompile(`(?i)x509|certificate|tls:`), "HTTPS certificate problem", "If you just connected the domain, wait 1-5 minutes. Otherwise check ports 80/443 are open and no AAAA record points elsewhere.", "https-ssl"},
	{regexp.MustCompile(`(?i)no such host|nxdomain|server misbehaving`), "The domain name doesn't exist in DNS", "Check the spelling, that the domain hasn't expired, and that its A record exists at your registrar.", "dns-waiting"},
	{regexp.MustCompile(`(?i)does not point|points to|dns not ready|wrong ip`), "The domain points somewhere else", "Change the A record at your registrar to your server's IP and delete other A/AAAA records for that name.", "dns-waiting"},
	{regexp.MustCompile(`(?i)\b429\b|rate.?limit|too many requests`), "Too many requests", "A service asked Rover to slow down. Wait a few minutes and try again.", "error-rate-limit"},
	{regexp.MustCompile(`(?i)repository not found|could not read from remote|private|\b404\b.*(git|repo)|authentication required`), "The repository is private or the link is wrong", "Click Connect account next to the Git link and add a read-only token; check the branch name.", "error-git-private"},
	{regexp.MustCompile(`(?i)deadline exceeded|timed? ?out|timeout`), "The server took too long to answer", "It may be busy or a firewall is blocking it. Wait a minute and try again; check CPU/RAM in your provider panel.", "error-timeout"},
	{regexp.MustCompile(`(?i)\b50[234]\b|bad gateway|service unavailable`), "The site's web server isn't answering", "Restart the site. If it keeps happening, roll back to the last good deploy.", "site-down"},
}

func explainError(msg string) (title, text, kb string) {
	for _, p := range errPatterns {
		if p.re.MatchString(msg) {
			return p.title, p.text, p.kb
		}
	}
	return "Something went wrong", "Rover's message: " + strings.TrimSpace(msg) + ". Try again; if it repeats, roll back to the last working deploy.", ""
}

// diag hooks (tests replace them; the real ones use the network).
var (
	diagStatuses = func(c *Config) (map[string]CApp, string) { return appStatuses(c) }
	diagLookup   = func(host string) ([]string, error) {
		hc := checkHost(host, "")
		var out []string
		for _, ips := range hc.Found {
			for _, ip := range ips {
				if !contains(out, ip) {
					out = append(out, ip)
				}
			}
		}
		if len(out) == 0 && hc.Message != "" {
			return nil, fmt.Errorf("%s", hc.Message)
		}
		return out, nil
	}
	diagRegistrar = func(domain string) DNSHost { return detectDNSHost(domain) }
	diagConnected = func() bool { return sshS.connected() }
)

func tcall(name string, args map[string]any) *ToolCall { return &ToolCall{Name: name, Args: args} }

// diagnoseServer covers the parts every site depends on.
func diagnoseServer(c *Config) []Finding {
	var f []Finding
	if c.ServerIP == "" {
		return append(f, Finding{Level: "fail", Title: "No server connected", Text: "Your websites need a server. Connect a hosting account or type your VPS's IP.", Href: "#/hosting", FixLbl: "Connect a server", KB: "first-steps"})
	}
	if !diagConnected() {
		f = append(f, Finding{Level: "warn", Title: "Not connected to " + serverLabel(c, c.ActiveServer), Text: "Rover can't reach the server right now. Deploys and restarts wait until it reconnects. Check that it's running in your provider panel.", Href: "#/settings/server", FixLbl: "Server settings", KB: "ssh-failed"})
	}
	if c.CoolifyToken == "" {
		f = append(f, Finding{Level: "fail", Title: "The hosting engine isn't set up", Text: "Rover needs to install Coolify on the server once (about 5-10 minutes, automatic).", Fix: tcall("setup_server", map[string]any{}), FixLbl: "Set up the server", KB: "use-server"})
	}
	return f
}

// diagnoseSite explains why a project is down or its domain doesn't work.
func diagnoseSite(c *Config, s *Site, deep bool) Diagnosis {
	d := Diagnosis{Site: s.Slug, Name: s.Name}
	if !onActive(c, s) {
		d.Findings = append(d.Findings, Finding{Level: "info", Title: "On another server", Text: fmt.Sprintf("%s is hosted on %s. Switch to that server to check it closely.", s.Name, serverLabel(c, s.Server)), Fix: tcall("switch_server", map[string]any{"server": s.Server}), FixLbl: "Switch server", KB: "multiple-servers"})
		d.Summary = d.Findings[0].Text
		return d
	}
	d.Findings = append(d.Findings, diagnoseServer(c)...)
	serverOK := len(d.Findings) == 0
	if s.LastDeploy == "" {
		d.Findings = append(d.Findings, Finding{Level: "info", Title: "Nothing published yet", Text: "This project has no files online. Drop a folder or zip on the project page.", Href: "#/project/" + s.Slug, FixLbl: "Deploy files", KB: "new-project"})
	}
	if ld := lastDeployOf(c, s); ld != nil && ld.Status == "failed" {
		t, x, kb := explainError(ld.Error)
		f := Finding{Level: "fail", Title: "Last deploy failed: " + t, Text: x + " The previous version is still live.", KB: kb}
		if strings.Contains(kb, "index") || strings.Contains(kb, "build") {
			f.Href, f.FixLbl = "#/project/"+s.Slug+"/files", "Open files"
		} else {
			f.Fix, f.FixLbl = tcall("deploy_folder", map[string]any{"site": s.Slug}), "Try the deploy again"
		}
		if ld.Error != "" {
			f.Text += " (Error: " + clip(ld.Error, 160) + ")"
		}
		d.Findings = append(d.Findings, f)
	}
	if serverOK && s.LastDeploy != "" {
		status, apiErr := diagStatuses(c)
		st := siteStatus(s, status, apiErr)
		switch {
		case apiErr != "":
			t, x, kb := explainError(apiErr)
			d.Findings = append(d.Findings, Finding{Level: "warn", Title: "The hosting engine didn't answer: " + t, Text: x, KB: kb, Href: "#/settings/server", FixLbl: "Server settings"})
		case liveState(st) == "down":
			d.Findings = append(d.Findings, Finding{Level: "fail", Title: "The site's web server is stopped (" + st + ")", Text: "Restarting usually fixes it. If it stops again, roll back to the previous deploy.", Fix: tcall("redeploy_site", map[string]any{"site": s.Slug}), FixLbl: "Restart it", KB: "site-down"})
		case liveState(st) == "busy":
			d.Findings = append(d.Findings, Finding{Level: "info", Title: "The site is starting (" + st + ")", Text: "Give it a minute, then check again."})
		}
	}
	if h, ok := healthSnapshot()[s.Slug]; ok && !h.OK && s.LastDeploy != "" {
		t, x, kb := explainError(h.Error + fmt.Sprintf(" %d", h.Code))
		d.Findings = append(d.Findings, Finding{Level: "fail", Title: "Not answering at " + h.URL + ": " + t, Text: x, KB: kb, Fix: tcall("run_health", map[string]any{"site": s.Slug}), FixLbl: "Check again"})
	}
	if s.Domain != "" {
		d.Findings = append(d.Findings, diagnoseDomain(c, s, deep)...)
	}
	if s.Git != nil && s.Git.LastError != "" {
		t, x, kb := explainError(s.Git.LastError)
		d.Findings = append(d.Findings, Finding{Level: "warn", Title: "Couldn't get the latest code: " + t, Text: x, KB: kb, Fix: tcall("pull_and_deploy", map[string]any{"site": s.Slug}), FixLbl: "Try again"})
	}
	d.Summary = summarize(d)
	return d
}

func diagnoseDomain(c *Config, s *Site, deep bool) []Finding {
	var f []Finding
	if s.DomainLive && s.HTTPSActive {
		return f
	}
	if s.DomainLive {
		return append(f, Finding{Level: "info", Title: s.Domain + " works; the padlock (HTTPS) is on its way", Text: "Certificates usually arrive within 1-5 minutes. If it takes more than 30 minutes, check that ports 80 and 443 are open.", KB: "https-ssl"})
	}
	if !deep {
		return append(f, Finding{Level: "warn", Title: s.Domain + " doesn't point to your server yet", Text: "Ask me to check the domain and I'll look at what the internet sees right now.", Fix: tcall("check_dns", map[string]any{"site": s.Slug}), FixLbl: "Check the domain", KB: "dns-waiting"})
	}
	ips, err := diagLookup(s.Domain)
	want := c.ServerIP
	reg := diagRegistrar(s.Domain)
	where := reg.Name
	switch {
	case err != nil || len(ips) == 0:
		f = append(f, Finding{Level: "fail", Title: s.Domain + " has no A record yet", Text: fmt.Sprintf("The internet can't find an address for %s. At %s add an A record: name @, value %s.", s.Domain, where, want), Href: "#/project/" + s.Slug + "/domain", FixLbl: "Open the domain steps", KB: kbForRegistrar(reg.ID)})
	case !contains(ips, want):
		extra := ""
		if reg.ID == "godaddy" {
			extra = " On GoDaddy this is usually the A record \"@\" that says Parked - edit it, and remove any Forwarding."
		}
		f = append(f, Finding{Level: "fail", Title: s.Domain + " points to " + strings.Join(ips, ", ") + " (not your server)", Text: fmt.Sprintf("Change the A record for %s at %s to %s and delete the other A/AAAA records for that name.%s After that it usually works within an hour.", s.Domain, where, want, extra), Href: "#/project/" + s.Slug + "/domain", FixLbl: "Open the domain steps", KB: kbForRegistrar(reg.ID)})
	case len(ips) > 1:
		f = append(f, Finding{Level: "warn", Title: s.Domain + " has extra addresses", Text: "It points to your server but also to " + strings.Join(without(ips, want), ", ") + ". Delete the extra A records so every visitor reaches your server.", Href: "#/project/" + s.Slug + "/domain", FixLbl: "Open the domain steps", KB: "dns-waiting"})
	default:
		f = append(f, Finding{Level: "ok", Title: s.Domain + " points to your server", Text: "Rover will connect it at the next check (within a few minutes). You can also open the Domain tab and click Check now.", Href: "#/project/" + s.Slug + "/domain", FixLbl: "Check now", KB: "dns-waiting"})
	}
	if len(reg.Nameservers) > 0 && s.PendingNS != nil && len(s.PendingNS) > 0 {
		f = append(f, Finding{Level: "info", Title: "Waiting for the nameserver change", Text: "The registrar still shows " + strings.Join(reg.Nameservers, ", ") + ". Nameserver changes can take a few hours.", KB: "cloudflare"})
	}
	return f
}

func kbForRegistrar(id string) string {
	switch id {
	case "godaddy":
		return "godaddy"
	case "cloudflare":
		return "cloudflare"
	case "namecheap", "porkbun", "hostinger", "squarespace":
		return "other-registrars"
	}
	return "dns-waiting"
}

func without(list []string, x string) []string {
	var out []string
	for _, v := range list {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func summarize(d Diagnosis) string {
	var bad []string
	for _, f := range d.Findings {
		if f.Level == "fail" || f.Level == "warn" {
			bad = append(bad, f.Title)
		}
	}
	name := firstNonEmpty(d.Name, "Everything")
	switch len(bad) {
	case 0:
		return name + " looks healthy: I found no problems."
	case 1:
		return "I found one problem with " + name + ": " + bad[0] + "."
	}
	return fmt.Sprintf("I found %d things to fix for %s. Start with the first one.", len(bad), name)
}

// diagnoseAll is the "what needs my attention?" check across projects.
func diagnoseAll(c *Config) Diagnosis {
	d := Diagnosis{Name: "your sites"}
	d.Findings = append(d.Findings, diagnoseServer(c)...)
	if c.ServerIP != "" && c.CoolifyToken != "" && diagConnected() {
		status, apiErr := diagStatuses(c)
		for _, s := range c.Sites {
			if s.Archived || !onActive(c, s) || s.LastDeploy == "" {
				continue
			}
			if ld := lastDeployOf(c, s); ld != nil && ld.Status == "failed" {
				t, _, kb := explainError(ld.Error)
				d.Findings = append(d.Findings, Finding{Level: "fail", Title: s.Name + ": last deploy failed (" + t + ")", Text: "The previous version is still live. Ask me \"why did " + s.Slug + " fail?\" for details.", Fix: tcall("diagnose", map[string]any{"site": s.Slug}), FixLbl: "Diagnose", KB: kb})
			}
			if apiErr == "" && liveState(siteStatus(s, status, apiErr)) == "down" {
				d.Findings = append(d.Findings, Finding{Level: "fail", Title: s.Name + " is stopped", Text: "Restarting usually fixes it.", Fix: tcall("redeploy_site", map[string]any{"site": s.Slug}), FixLbl: "Restart it", KB: "site-down"})
			}
			if s.Domain != "" && !s.DomainLive {
				d.Findings = append(d.Findings, Finding{Level: "warn", Title: s.Domain + " isn't pointing to your server yet", Text: "Ask me to check it and I'll tell you what to change.", Fix: tcall("check_dns", map[string]any{"site": s.Slug}), FixLbl: "Check the domain", KB: "dns-waiting"})
			}
		}
	}
	if len(c.Sites) == 0 && c.ServerIP != "" {
		d.Findings = append(d.Findings, Finding{Level: "info", Title: "No projects yet", Text: "Add your first website: drag a folder in or pick a template.", Href: "#/new", FixLbl: "Add new project", KB: "new-project"})
	}
	d.Summary = summarize(d)
	return d
}

// explainLog picks the important lines out of a container/deploy log.
func explainLog(raw string) []Finding {
	var f []Finding
	seen := map[string]bool{}
	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0 && len(f) < 4; i-- {
		l := strings.TrimSpace(lines[i])
		low := strings.ToLower(l)
		if l == "" || !(strings.Contains(low, "error") || strings.Contains(low, "fail") || strings.Contains(low, "emerg") || strings.Contains(low, "denied") || strings.Contains(low, " 50") || strings.Contains(low, "refused") || strings.Contains(low, "no such file")) {
			continue
		}
		t, x, kb := explainError(l)
		if strings.Contains(low, "no such file") || strings.Contains(low, "open()") && strings.Contains(low, "failed") {
			t, x, kb = "A file visitors asked for is missing", "Someone opened an address that has no file. Check links and file names (online they are case-sensitive).", "old-content"
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		f = append(f, Finding{Level: "warn", Title: t, Text: x + " Log line: " + clip(redact(l, false), 180), KB: kb})
	}
	if len(f) == 0 {
		f = append(f, Finding{Level: "ok", Title: "No errors in the recent log", Text: "The last lines look normal."})
	}
	return f
}
