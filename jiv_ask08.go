package main

import (
	"context"
	"errors"
	"regexp"
)

var (
	reShowMe = regexp.MustCompile(`\b(show me|guide me|walk me|teach me|step by step|walkthrough|walk-through|show how|dikhao|sikhao|batao kaise)\b`)
	reHowTo  = regexp.MustCompile(`\b(how (do|can|to|should) |how do i|kaise)`)
	// single steps the classic pipeline already handles as one action card
	classicTools = map[string]bool{"diagnose": true, "check_dns": true, "explain_logs": true, "rollback": true, "restart_site": true, "deploy_folder": true, "backup_now": true, "create_project": true}
)

// jivAsk08 handles walkthroughs and plans; false means "carry on with the
// classic answer".
func jivAsk08(ctx context.Context, send func(map[string]any), c *Config, req askReq, in Intent, lang string) bool {
	q := req.Q
	t := jivNormalize(q)
	explicit := reShowMe.MatchString(t)
	if explicit || reHowTo.MatchString(t) {
		if g, _ := matchGuide(q); g != nil {
			ref := map[string]any{"id": g.ID, "title": g.Title, "auto": explicit, "site": req.Site}
			if explicit {
				msg := "Let's do it together. I'll highlight each button; do the step and I'll move on by myself."
				if lang == "hi" {
					msg = "चलिए साथ में करते हैं। मैं हर बटन दिखाऊँगा; आप कदम करें, मैं अपने आप आगे बढ़ूँगा।"
				}
				send(map[string]any{"t": "text", "v": msg})
				send(map[string]any{"t": "guide", "guide": ref})
				return true
			}
			send(map[string]any{"t": "guide", "guide": ref})
			return false // the article answer follows, with a "Show me how" button
		}
		if explicit && in.Call == nil {
			return false
		}
	}
	if in.Question && !reWhyDeploy.MatchString(t) && !reDiskCheck.MatchString(t) {
		return false
	}
	d := planFromText(c, q, req.Site)
	if len(d.steps) == 0 {
		return false
	}
	if len(d.steps) == 1 && classicTools[d.steps[0].Tool] {
		return false
	}
	if d.needSite {
		var opts []map[string]string
		for _, s := range activeSites(c) {
			opts = append(opts, map[string]string{"slug": s.Slug, "name": s.Name})
		}
		if len(opts) == 0 {
			send(map[string]any{"t": "text", "v": "You don't have any projects yet. Create one first (ask: show me how to add a project)."})
			return true
		}
		send(map[string]any{"t": "text", "v": jt("which", lang)})
		send(map[string]any{"t": "plansite", "q": q, "options": opts})
		return true
	}
	p, _, err := makePlan(ctx, c, q, req.Site, false)
	if err != nil {
		if errors.Is(err, errNeedSite) {
			return false
		}
		send(map[string]any{"t": "text", "v": "I can't plan that yet: " + err.Error() + "."})
		return true
	}
	readOnly := true
	for _, s := range p.Steps {
		if !s.Read {
			readOnly = false
		}
	}
	if readOnly {
		// nothing changes, so it runs straight away; any fix Jiv adds
		// later still needs a click
		planUpdate(func() { p.State, p.Auto = "running", true })
		startPlan(p)
		send(map[string]any{"t": "text", "v": "Let me check that for you (read-only: nothing is changed)."})
	} else {
		msg := "Here's my plan. Press Run to start; steps marked ⚠ ask again before they run."
		if lang == "hi" {
			msg = "यह मेरा प्लान है। शुरू करने के लिए Run दबाएँ; ⚠ वाले कदम चलने से पहले फिर पूछेंगे।"
		}
		send(map[string]any{"t": "text", "v": msg})
	}
	for _, n := range p.Notes {
		send(map[string]any{"t": "note", "v": n})
	}
	send(map[string]any{"t": "plan", "plan": p.snapshot()})
	return true
}
