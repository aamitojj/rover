package main

// Intent detection for Jiv without a model: maps plain requests
// ("roll back bakery", "meri site band kyun hai", "add domain x.com to
// shop") to actions or diagnoses. Questions ("how do I roll back?") go
// to the knowledge base instead, with the action offered as a suggestion.

import (
	"regexp"
	"strings"
)

type Intent struct {
	Call     *ToolCall `json:"call,omitempty"`
	Question bool      `json:"question"` // phrased as a question: answer first, offer the action
	NeedSite bool      `json:"need_site,omitempty"`
	Diagnose bool      `json:"diagnose,omitempty"`
}

var (
	reQuestion  = regexp.MustCompile(`^(how|what|where|when|which|who|is|are|does|do|should|why|kaise|kya|kahan|kab|kaun|क्या|कैसे|कहाँ)\b|\?\s*$`)
	reRequest   = regexp.MustCompile(`^(please |pls |plz |jiv,? )?(can|could|would) you\b|^(please|pls|plz)\b`)
	reDomain    = regexp.MustCompile(`\b((?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,24})\b`)
	reWhy       = regexp.MustCompile(`\b(why|kyun|kyon|kyu|what'?s wrong|whats wrong|what is wrong|diagnose|troubleshoot|problem|kya hua|kya problem|गड़बड़|क्यों)\b`)
	reBroken    = regexp.MustCompile(`\b(down|notwork|not work|error|fail\w*|broken|crash\w*|offline|stopped|band|slow|502|503|404|wrong|nahi|नहीं|बंद)\b`)
	reDomainW   = regexp.MustCompile(`\b(domain|dns|https|ssl|certificate|padlock|nameserver|arecord|डोमेन)\b`)
	reAttention = regexp.MustCompile(`\b(what needs (my )?attention|anything wrong|is everything (ok|okay|fine|working)|status of (my )?(sites|projects|everything)|sab theek|sab thik|सब ठीक)\b`)
	reHealth    = regexp.MustCompile(`\b(check|test|ping|scan)\b.*\b(all|every|my)?\s*(sites?|websites?|projects?|uptime|health)\b|\bhealth ?check\b|\buptime\b`)
	reLogs      = regexp.MustCompile(`\b(explain|read|show|check|samjhao|dekho)\b.*\blogs?\b|\blogs?\b.*\b(explain|mean|samjhao)\b`)
	reRollback  = regexp.MustCompile(`\b(rollback|revert|undo (the )?(last )?deploy|previous version|old version|purana version|पुराना)\b`)
	reRestart   = regexp.MustCompile(`\b(restart|reboot|redeploy|re-deploy|reload)\b`)
	rePull      = regexp.MustCompile(`\b(pull|latest commit|update from git|from github|latest code)\b`)
	reDeploy    = regexp.MustCompile(`\b(deploy|publish|upload|go live|push live|make (it )?live)\b`)
	reCreate    = regexp.MustCompile(`\b(create|make|new|add|start|banao|banaye|बनाओ)\b.*\b(project|site|website)\b|\bnew (project|site|website)\b`)
	reCalled    = regexp.MustCompile(`\b(?:called|named|name|naam|नाम)\s+["']?([a-z0-9][a-z0-9 _-]{0,38})["']?`)
	reAddDomain = regexp.MustCompile(`\b(add|connect|use|set|attach|point|link|jodo|lagao)\b`)
	reCheckDNS  = regexp.MustCompile(`\b(check|test|verify|look at|dekho)\b.*\b(domain|dns|nameserver)\b`)
	reBackup    = regexp.MustCompile(`\b(back ?up|backup) (now|everything|all|my)\b|\b(take|make|run|do) a? ?backup\b|\bbackup (lo|karo|le lo)\b`)
	reAuto      = regexp.MustCompile(`\bauto[- ]?deploy\b`)
	reOff       = regexp.MustCompile(`\b(off|disable|stop|turn off|band)\b`)
	reSwitchSrv = regexp.MustCompile(`\b(switch|change|use|move to)\b.*\bserver\b|\bserver\b.*\b(switch|badlo)\b`)
	reOpen      = regexp.MustCompile(`^(please )?(open|go to|goto|show( me)?|take me to|navigate to|kholo|dikhao|खोलो)\b|\b(kholo|dikhao|खोलो)$`)
	reFolder    = regexp.MustCompile(`\b(folder|explorer)\b`)
	reBrain     = regexp.MustCompile(`\b(download|install|get|setup|set up)\b.*\b(brain|offline (ai|model)|local (ai|model))\b|\boffline brain\b`)
	reHostAcc   = regexp.MustCompile(`\b(connect|add|link|jodo)\b.*\b(hetzner|digitalocean|vultr|linode|akamai|lightsail|aws|oracle|contabo|hostinger|scaleway|upcloud|hosting account|provider)\b`)
	reSetupSrv  = regexp.MustCompile(`\b(set ?up|install|prepare)\b.*\b(server|coolify|engine)\b`)
)

var pageWords = []struct {
	re   *regexp.Regexp
	page string
}{
	{regexp.MustCompile(`hosting|accounts?|providers?`), "hosting"},
	{regexp.MustCompile(`dashboard|home`), "dashboard"},
	{regexp.MustCompile(`new project|add (a )?new|add project`), "new-project"},
	{regexp.MustCompile(`templates?`), "templates"},
	{regexp.MustCompile(`deploys?|history`), "deploys"},
	{regexp.MustCompile(`domains?`), "domains"},
	{regexp.MustCompile(`testing|app test|preview`), "testing"},
	{regexp.MustCompile(`jiv settings|brain|offline`), "jiv-settings"},
	{regexp.MustCompile(`server settings|server`), "server"},
	{regexp.MustCompile(`connect ai|ai settings|ai`), "connect-ai"},
	{regexp.MustCompile(`storage`), "storage"},
	{regexp.MustCompile(`backups?|alerts?`), "backups"},
	{regexp.MustCompile(`updates?`), "updates"},
	{regexp.MustCompile(`groups?|tags?`), "organise"},
	{regexp.MustCompile(`guide|help|docs?`), "guide"},
	{regexp.MustCompile(`shortcuts?|keyboard`), "shortcuts"},
	{regexp.MustCompile(`settings?`), "settings"},
	{regexp.MustCompile(`projects?|sites?`), "projects"},
}

var providerWords = map[string]string{"hetzner": "hetzner", "digitalocean": "digitalocean", "vultr": "vultr", "linode": "linode", "akamai": "linode", "lightsail": "lightsail", "aws": "lightsail",
	"oracle": "oracle", "contabo": "contabo", "hostinger": "hostinger", "scaleway": "scaleway", "upcloud": "upcloud"}

// findSiteIn returns the project the text talks about (slug, name or
// domain, with small typos allowed).
func findSiteIn(c *Config, text string) *Site {
	low := " " + strings.ToLower(text) + " "
	var best *Site
	bestLen := 0
	for _, s := range c.Sites {
		for _, k := range []string{s.Slug, strings.ToLower(s.Name), strings.ToLower(s.Domain)} {
			if len(k) < 2 {
				continue
			}
			if i := strings.Index(low, k); i >= 0 {
				before, after := low[i-1], low[i+len(k)]
				if !isWordByte(before) && !isWordByte(after) && len(k) > bestLen {
					best, bestLen = s, len(k)
				}
			}
		}
	}
	if best != nil {
		return best
	}
	for _, w := range jivWords(low) {
		if len(w) < 5 {
			continue
		}
		for _, s := range c.Sites {
			if len(s.Slug) >= 5 && editDistance(w, s.Slug, 1) <= 1 {
				return s
			}
		}
	}
	return nil
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' || b == '_'
}

// detectIntent never returns an action for a project that doesn't exist.
func detectIntent(c *Config, q string) Intent {
	raw := strings.TrimSpace(q)
	t := jivNormalize(raw)
	in := Intent{Question: reQuestion.MatchString(t) && !reRequest.MatchString(t)}
	site := findSiteIn(c, raw)
	slug := ""
	if site != nil {
		slug = site.Slug
	} else if len(c.Sites) == 1 {
		slug = c.Sites[0].Slug
	}
	withSite := func(name string, extra map[string]any) Intent {
		args := map[string]any{}
		for k, v := range extra {
			args[k] = v
		}
		if slug != "" {
			args["site"] = slug
		} else {
			in.NeedSite = true
		}
		in.Call = &ToolCall{Name: name, Args: args}
		return in
	}
	// Diagnosis first: "why is X down", "my domain doesn't work".
	if reAttention.MatchString(t) {
		in.Diagnose, in.Call = true, &ToolCall{Name: "diagnose", Args: map[string]any{}}
		return in
	}
	if (reWhy.MatchString(t) || !in.Question && reBroken.MatchString(t) && (site != nil || strings.Contains(t, "site") || strings.Contains(t, "my "))) && reBroken.MatchString(t) {
		in.Diagnose = true
		domQ := reDomainW.MatchString(t) || (site != nil && site.Domain != "" && strings.Contains(strings.ToLower(raw), strings.ToLower(site.Domain)))
		if domQ && (site == nil || site.Domain != "") && slug != "" {
			if s := findSite(c, slug); s != nil && s.Domain != "" {
				in.Call = &ToolCall{Name: "check_dns", Args: map[string]any{"site": slug}}
				return in
			}
		}
		if domQ && site == nil {
			for _, s := range c.Sites {
				if s.Domain != "" && !s.DomainLive {
					in.Call = &ToolCall{Name: "check_dns", Args: map[string]any{"site": s.Slug}}
					return in
				}
			}
		}
		args := map[string]any{}
		if site != nil {
			args["site"] = site.Slug
		} else if len(c.Sites) == 1 {
			args["site"] = c.Sites[0].Slug
		}
		in.Call = &ToolCall{Name: "diagnose", Args: args}
		return in
	}
	if reLogs.MatchString(t) && !in.Question {
		return withSite("explain_logs", nil)
	}
	if reBrain.MatchString(t) && !in.Question {
		in.Call = &ToolCall{Name: "install_brain", Args: map[string]any{}}
		return in
	}
	if m := reHostAcc.FindStringSubmatch(t); m != nil && !in.Question {
		in.Call = &ToolCall{Name: "add_hosting_account", Args: map[string]any{}}
		for w, id := range providerWords {
			if strings.Contains(t, w) {
				in.Call.Args["provider"] = id
			}
		}
		return in
	}
	if reHealth.MatchString(t) && !reDomainW.MatchString(t) && !in.Question {
		args := map[string]any{}
		if site != nil {
			args["site"] = site.Slug
		}
		in.Call = &ToolCall{Name: "run_health", Args: args}
		return in
	}
	if reCheckDNS.MatchString(t) && !in.Question {
		return withSite("check_dns", nil)
	}
	if reBackup.MatchString(t) && !in.Question {
		in.Call = &ToolCall{Name: "backup_now", Args: map[string]any{}}
		return in
	}
	if reSwitchSrv.MatchString(t) && len(c.Servers) > 1 {
		for _, p := range c.Servers {
			if strings.Contains(t, strings.ToLower(p.Label)) || strings.Contains(t, p.ServerIP) || strings.Contains(t, strings.ToLower(strings.Fields(p.Label + " x")[0])) {
				if p.ID != c.ActiveServer {
					in.Call = &ToolCall{Name: "switch_server", Args: map[string]any{"server": p.ID}}
					return in
				}
			}
		}
	}
	if reAuto.MatchString(t) && site != nil {
		return withSite("toggle_auto_deploy", map[string]any{"on": !reOff.MatchString(t)})
	}
	if reCreate.MatchString(t) && !in.Question {
		name := ""
		if m := reCalled.FindStringSubmatch(t); m != nil {
			name = strings.TrimSpace(m[1])
		}
		if name != "" {
			in.Call = &ToolCall{Name: "create_site", Args: map[string]any{"name": name}}
		} else {
			in.Call = &ToolCall{Name: "open_page", Args: map[string]any{"page": "new-project"}}
		}
		return in
	}
	if d := reDomain.FindString(t); d != "" && reAddDomain.MatchString(t) && reDomainW.MatchString(t+" domain") && !isSiteDomain(c, d) && !strings.HasSuffix(d, ".html") {
		return withSite("set_domain", map[string]any{"domain": d})
	}
	if reRollback.MatchString(t) {
		return withSite("rollback_site", nil)
	}
	if rePull.MatchString(t) && site != nil && site.Git != nil {
		return withSite("pull_and_deploy", nil)
	}
	if reRestart.MatchString(t) {
		return withSite("redeploy_site", nil)
	}
	if reSetupSrv.MatchString(t) && !in.Question {
		in.Call = &ToolCall{Name: "setup_server", Args: map[string]any{}}
		return in
	}
	if reDeploy.MatchString(t) && site != nil {
		return withSite("deploy_folder", nil)
	}
	if reOpen.MatchString(t) {
		if site != nil {
			if reFolder.MatchString(t) {
				return withSite("open_project_folder", nil)
			}
			tab := ""
			for _, x := range []string{"files", "deploys", "domain", "logs", "settings"} {
				if strings.Contains(t, strings.TrimSuffix(x, "s")) {
					tab = x
				}
			}
			args := map[string]any{"site": site.Slug}
			if tab != "" {
				args["tab"] = tab
			}
			in.Call = &ToolCall{Name: "open_project", Args: args}
			return in
		}
		rest := reOpen.ReplaceAllString(t, "")
		for _, p := range pageWords {
			if p.re.MatchString(rest) {
				in.Call = &ToolCall{Name: "open_page", Args: map[string]any{"page": p.page}}
				return in
			}
		}
	}
	return in
}

func isSiteDomain(c *Config, d string) bool {
	for _, s := range c.Sites {
		if s.Domain == d {
			return true
		}
	}
	return false
}
