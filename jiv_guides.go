package main

// Rover 0.8: Jiv's guide mode ("Show me how"). A guide is a list of steps
// that highlight the real button or field, say what to do in one or two
// plain sentences, wait for the user and detect when the step is done.
// Steps on other websites (Oracle console, GoDaddy DNS, Cloudflare tokens)
// are checklists with an embedded illustration, copy buttons and a check
// that runs when the user comes back to Rover.
//
// Guides are data (jivguides/*.json, embedded in the exe) and work with no
// model. With the offline brain, Jiv can also build a guide for a free-form
// question from the same catalogue of real buttons (anchors); the model can
// only choose anchors that exist, so it cannot invent buttons.
//
// Checks are read-only: they look at Rover's own state, DNS and HTTPS.

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed jivguides/*.json
var jivGuideFS embed.FS

type Guide struct {
	ID        string      `json:"id"`
	Group     string      `json:"group,omitempty"`
	Title     string      `json:"title"`
	TitleHi   string      `json:"title_hi,omitempty"`
	Q         []string    `json:"q,omitempty"`
	Intro     string      `json:"intro,omitempty"`
	IntroHi   string      `json:"intro_hi,omitempty"`
	KB        string      `json:"kb,omitempty"`
	Needs     string      `json:"needs,omitempty"` // "site": the guide works on one project
	Do        string      `json:"do,omitempty"`    // request Jiv can plan instead ("Do it for me")
	Steps     []GuideStep `json:"steps"`
	Generated bool        `json:"generated,omitempty"`
	Site      string      `json:"site,omitempty"` // resolved project
}

type GuideStep struct {
	Kind     string      `json:"kind"`         // ui | external | info
	Go       string      `json:"go,omitempty"` // screen to open when the user is elsewhere
	At       string      `json:"at,omitempty"` // regex the address must match (default: Go)
	Sel      string      `json:"sel,omitempty"`
	Text     string      `json:"text,omitempty"` // the element must contain this text
	Say      string      `json:"say"`
	SayHi    string      `json:"say_hi,omitempty"`
	Done     *GuideDone  `json:"done,omitempty"`
	Optional bool        `json:"optional,omitempty"`
	SkipIf   string      `json:"skip_if,omitempty"` // check: the step is skipped when it passes
	Site     string      `json:"site,omitempty"`    // external: the website's name
	URL      string      `json:"url,omitempty"`
	Image    string      `json:"image,omitempty"` // web/guides/*.svg
	Items    []GuideItem `json:"items,omitempty"`
	Verify   string      `json:"verify,omitempty"` // check run when the user returns
}

type GuideItem struct {
	Text  string `json:"text"`
	Copy  string `json:"copy,omitempty"`
	Label string `json:"label,omitempty"`
}

type GuideDone struct {
	Click   bool   `json:"click,omitempty"`   // the highlighted element was clicked
	Route   string `json:"route,omitempty"`   // the address matches
	Exists  string `json:"exists,omitempty"`  // a visible element appeared
	Filled  string `json:"filled,omitempty"`  // an input has a value
	Check   string `json:"check,omitempty"`   // a server check passes
	Changed bool   `json:"changed,omitempty"` // with Check: its value changed since the step began
}

var (
	guidesOnce sync.Once
	guideList  []*Guide
	guideIdx   *kbIndex
)

func loadGuides() []*Guide {
	guidesOnce.Do(func() {
		ents, _ := jivGuideFS.ReadDir("jivguides")
		var docs []*KBArticle
		for _, e := range ents {
			b, err := jivGuideFS.ReadFile("jivguides/" + e.Name())
			if err != nil {
				continue
			}
			var gs []*Guide
			if err := json.Unmarshal(b, &gs); err != nil {
				panic("jivguides/" + e.Name() + ": " + err.Error())
			}
			guideList = append(guideList, gs...)
		}
		for _, g := range guideList {
			docs = append(docs, &KBArticle{ID: g.ID, Title: g.Title, Q: g.Q, Body: g.Intro})
		}
		guideIdx = buildKBIndex(docs)
	})
	return guideList
}

func guideByID(id string) *Guide {
	for _, g := range loadGuides() {
		if g.ID == id {
			return g
		}
	}
	return nil
}

// matchGuide finds the guide for a question ("show me how to add a domain").
func matchGuide(q string) (*Guide, float64) {
	loadGuides()
	t := jivNormalize(q)
	if reDefinition.MatchString(t) && !reShowMe.MatchString(t) {
		return nil, 0 // "what is hosting?" wants an explanation, not a walkthrough
	}
	q = stripGuideWords(t)
	hits := guideIdx.Search(q, "", 3)
	if len(hits) == 0 || hits[0].Score < 2.2 || hits[0].Cover < 0.4 {
		return nil, 0
	}
	return guideByID(hits[0].ID), hits[0].Score
}

type guideRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Do    string `json:"do,omitempty"`
}

// ---------- resolving a guide for this PC ----------

func guideVars(c *Config, s *Site) map[string]string {
	v := map[string]string{"ip": c.ServerIP, "domain": "your-domain.com", "site": "", "siteName": "your project", "www": "www.your-domain.com", "pubkey": ""}
	if s != nil {
		v["site"], v["siteName"] = s.Slug, s.Name
		if s.Domain != "" {
			v["domain"], v["www"] = s.Domain, "www."+s.Domain
		}
		if p := findServer(c, s.Server); p != nil && p.ServerIP != "" {
			v["ip"] = p.ServerIP
		}
	}
	if v["ip"] == "" {
		v["ip"] = "your server's IP"
	}
	v["pubkey"] = firstNonEmpty(c.ManagedKeyPub, "(shown in Hosting accounts > Oracle after you connect once)")
	return v
}

var reGuideVar = regexp.MustCompile(`\{(ip|domain|site|siteName|www|pubkey)\}`)

func fillVars(s string, v map[string]string) string {
	return reGuideVar.ReplaceAllStringFunc(s, func(m string) string { return v[m[1:len(m)-1]] })
}

// resolveGuide fills in the project, domain and IP. Steps whose skip check
// already passes are dropped (e.g. "connect Cloudflare" when it is connected).
func resolveGuide(c *Config, g *Guide, site string) (*Guide, error) {
	var s *Site
	if site != "" {
		s = resolveSiteArg(c, site)
	}
	if g.Needs == "site" && s == nil {
		live := activeSites(c)
		if len(live) == 1 {
			s = live[0]
		} else {
			return nil, errNeedSite
		}
	}
	v := guideVars(c, s)
	out := *g
	if s != nil {
		out.Site = s.Slug
	}
	out.Title, out.Intro, out.TitleHi, out.IntroHi = fillVars(g.Title, v), fillVars(g.Intro, v), fillVars(g.TitleHi, v), fillVars(g.IntroHi, v)
	if s != nil {
		out.Do = strings.ReplaceAll(g.Do, "{site}", s.Name)
	}
	out.Steps = nil
	for _, st := range g.Steps {
		if st.SkipIf != "" {
			if r := runGuideCheck(c, fillVars(st.SkipIf, v)); r.OK {
				continue
			}
		}
		st.SkipIf = fillVars(st.SkipIf, v)
		st.Go, st.At, st.Sel, st.Text = fillVars(st.Go, v), fillVars(st.At, v), fillVars(st.Sel, v), fillVars(st.Text, v)
		st.Say, st.SayHi, st.URL, st.Verify = fillVars(st.Say, v), fillVars(st.SayHi, v), fillVars(st.URL, v), fillVars(st.Verify, v)
		if st.Done != nil {
			d := *st.Done
			d.Check, d.Route, d.Exists, d.Filled = fillVars(d.Check, v), fillVars(d.Route, v), fillVars(d.Exists, v), fillVars(d.Filled, v)
			st.Done = &d
		}
		items := make([]GuideItem, len(st.Items))
		for i, it := range st.Items {
			items[i] = GuideItem{Text: fillVars(it.Text, v), Copy: fillVars(it.Copy, v), Label: fillVars(it.Label, v)}
		}
		st.Items = items
		out.Steps = append(out.Steps, st)
	}
	return &out, nil
}

var errNeedSite = errors.New("which project?")

func activeSites(c *Config) []*Site {
	var out []*Site
	for _, s := range c.Sites {
		if !s.Archived {
			out = append(out, s)
		}
	}
	return out
}

// ---------- checks (read-only) ----------

type GuideCheck struct {
	OK      bool   `json:"ok"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message"`
}

// hooks for tests (the real ones use DNS and HTTPS)
var (
	guideDNSOK = func(host, ip string) (bool, string) {
		hc := checkHost(host, ip)
		return hc.OK, hc.Message
	}
	guideHTTPS = func(slug string) (bool, string) {
		res := runHealthFor([]string{slug})
		if h := res[slug]; h != nil {
			if h.OK && strings.HasPrefix(h.URL, "https://") {
				return true, "answers over HTTPS in " + strconv.FormatInt(h.MS, 10) + " ms"
			}
			if h.Error != "" {
				t, _, _ := explainError(h.Error)
				return false, t
			}
		}
		return false, "not answering over HTTPS yet"
	}
	guideTCP = func(addr string) bool {
		cn, err := net.DialTimeout("tcp", addr, 4*time.Second)
		if err == nil {
			cn.Close()
		}
		return err == nil
	}
)

func runGuideCheck(c *Config, spec string) GuideCheck {
	name, arg, _ := strings.Cut(spec, ":")
	site := func() *Site { return resolveSiteArg(c, arg) }
	yes := func(ok bool, good, bad string) GuideCheck {
		if ok {
			return GuideCheck{OK: true, Message: good}
		}
		return GuideCheck{Message: bad}
	}
	switch name {
	case "has_server":
		return yes(len(c.Servers) > 0, "A server is connected.", "No server connected yet.")
	case "server_connected":
		return yes(diagConnected(), "Rover is connected to your server.", "Not connected to the server yet.")
	case "engine_ready":
		return yes(c.CoolifyToken != "" || activeLite(c), "The server is set up.", "The server is not set up yet.")
	case "hosting_account":
		for _, a := range c.HostingAccounts {
			if arg == "" || a.Provider == arg {
				return GuideCheck{OK: true, Message: "Account connected: " + a.Nickname + "."}
			}
		}
		return GuideCheck{Message: "No " + firstNonEmpty(arg, "hosting") + " account connected yet."}
	case "home_server":
		for _, p := range c.Servers {
			if p.Kind == "home" {
				return GuideCheck{OK: true, Message: p.Label + " is added."}
			}
		}
		return GuideCheck{Message: "No home server yet."}
	case "site_count":
		return GuideCheck{OK: len(c.Sites) > 0, Value: strconv.Itoa(len(c.Sites)), Message: fmt.Sprintf("%d project(s).", len(c.Sites))}
	case "deploy_count":
		n := 0
		if dir, _, err := projectDir(arg); err == nil {
			if ix, err := readIndex(dir); err == nil {
				n = len(ix.Deploys)
			}
		}
		return GuideCheck{OK: n > 0, Value: strconv.Itoa(n), Message: fmt.Sprintf("%d deploy(s).", n)}
	case "release":
		s := site()
		if s == nil {
			return GuideCheck{Message: "unknown project"}
		}
		return GuideCheck{OK: s.Release != "", Value: s.Release, Message: "Live version: " + firstNonEmpty(s.Release, "none")}
	case "domain_set":
		s := site()
		return yes(s != nil && s.Domain != "", "The domain is saved.", "No domain saved yet.")
	case "dns_points":
		s := site()
		if s == nil || s.Domain == "" {
			return GuideCheck{Message: "Add the domain in Rover first."}
		}
		v := guideVars(c, s)
		ok, msg := guideDNSOK(s.Domain, v["ip"])
		if ok {
			return GuideCheck{OK: true, Message: s.Domain + " points to your server."}
		}
		return GuideCheck{Message: s.Domain + ": " + firstNonEmpty(msg, "does not point to "+v["ip"]+" yet") + ". DNS changes can take a few minutes."}
	case "domain_live":
		s := site()
		return yes(s != nil && s.DomainLive, "The domain is connected.", "The domain is not connected yet.")
	case "https_ok":
		s := site()
		if s == nil {
			return GuideCheck{Message: "unknown project"}
		}
		if s.HTTPSActive {
			return GuideCheck{OK: true, Message: "HTTPS works (padlock shown)."}
		}
		ok, msg := guideHTTPS(s.Slug)
		return GuideCheck{OK: ok, Message: msg}
	case "cf_connected":
		return yes(c.CloudflareToken != "", "Cloudflare is connected.", "Cloudflare is not connected yet.")
	case "has_replica":
		s := site()
		return yes(s != nil && len(s.Servers) > 0, "The project runs on more than one server.", "Only one server so far.")
	case "thispc_on":
		return yes(c.ThisPC != nil && c.ThisPC.Enabled, "This PC serves websites.", "Hosting on this PC is off.")
	case "brain_ready":
		return yes(brainAvailable(c), "Jiv's offline brain is installed.", "The offline brain is not installed yet.")
	case "ai_ready":
		return yes(brainAvailable(c) || len(c.AIConns) > 0 || strings.HasPrefix(c.AIDefault, "local:"), "An AI model is connected.", "No AI model connected yet.")
	case "update_checked":
		u := cachedUpdateInfo()
		if u == nil {
			return GuideCheck{Message: "Not checked yet."}
		}
		return GuideCheck{OK: true, Value: u.Latest, Message: "Checked: latest is " + firstNonEmpty(u.Latest, Version) + "."}
	case "tcp":
		return yes(guideTCP(arg), arg+" answers.", arg+" does not answer (port closed or server off).")
	}
	return GuideCheck{Message: "unknown check " + name}
}

func activeLite(c *Config) bool {
	p := findServer(c, c.ActiveServer)
	return p != nil && p.Engine == "lite"
}

// ---------- model-built guides ----------

// guideAnchor is a real button or field the model may point at.
type guideAnchor struct {
	ID   string `json:"id"`
	Go   string `json:"go"`
	At   string `json:"at,omitempty"`
	Sel  string `json:"sel"`
	Text string `json:"text,omitempty"`
	Desc string `json:"desc"`
}

func guideAnchors() []guideAnchor {
	seen := map[string]bool{}
	var out []guideAnchor
	for _, g := range loadGuides() {
		for i, st := range g.Steps {
			if st.Kind != "ui" || st.Sel == "" || st.Go == "" {
				continue
			}
			k := st.Go + "|" + st.Sel + "|" + st.Text
			if seen[k] {
				continue
			}
			seen[k] = true
			desc := st.Say
			if i := strings.Index(desc, ". "); i > 0 {
				desc = desc[:i+1]
			}
			out = append(out, guideAnchor{ID: fmt.Sprintf("%s.%d", g.ID, i+1), Go: st.Go, At: st.At, Sel: st.Sel, Text: st.Text, Desc: clip(desc, 110)})
		}
	}
	return out
}

// brainJSON asks the offline brain for JSON that must match schema
// (llama-server turns the schema into a grammar, so the reply always parses).
var brainJSON = func(ctx context.Context, system, user string, schema map[string]any, maxTokens int) (string, error) {
	c := snapshotConfig()
	if !brainAvailable(&c) {
		return "", errors.New("the offline brain is not installed")
	}
	base, key, err := brainSidecar.Ensure(ctx)
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"model": "jiv", "stream": false, "temperature": 0, "max_tokens": maxTokens,
		"messages":        []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "answer", "strict": true, "schema": schema}},
	}
	bs, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", strings.TrimSuffix(strings.TrimRight(base, "/"), "/v1")+"/v1/chat/completions", strings.NewReader(string(bs)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := (&http.Client{Timeout: 4 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error any `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if resp.StatusCode != 200 || len(out.Choices) == 0 {
		return "", fmt.Errorf("the offline brain answered %d", resp.StatusCode)
	}
	return out.Choices[0].Message.Content, nil
}

func generateGuide(ctx context.Context, c *Config, q, site string) (*Guide, error) {
	anchors := guideAnchors()
	ids := make([]string, len(anchors))
	var cat strings.Builder
	for i, a := range anchors {
		ids[i] = a.ID
		fmt.Fprintf(&cat, "%s: %s (screen %s)\n", a.ID, a.Desc, a.Go)
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"title", "steps"},
		"properties": map[string]any{
			"title": map[string]any{"type": "string", "maxLength": 80},
			"steps": map[string]any{"type": "array", "minItems": 1, "maxItems": 8, "items": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"anchor", "say"},
				"properties": map[string]any{"anchor": map[string]any{"type": "string", "enum": ids}, "say": map[string]any{"type": "string", "maxLength": 220}}}},
		},
	}
	sys := "You build short on-screen walkthroughs for Rover, a Windows app that publishes websites. Pick the buttons (anchors) the user must use, in order, and write one or two plain sentences for each. Only use anchors from this list:\n" + cat.String()
	raw, err := brainJSON(ctx, sys, q, schema, 600)
	if err != nil {
		return nil, err
	}
	var out struct {
		Title string
		Steps []struct{ Anchor, Say string }
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("the offline brain's walkthrough was not readable: %v", err)
	}
	byID := map[string]guideAnchor{}
	for _, a := range anchors {
		byID[a.ID] = a
	}
	g := &Guide{ID: "gen-" + randHex(4), Title: firstNonEmpty(strings.TrimSpace(out.Title), "Walkthrough"), Generated: true, Intro: "Jiv built this walkthrough for your question with the offline brain."}
	for _, st := range out.Steps {
		a, ok := byID[st.Anchor]
		if !ok || strings.TrimSpace(st.Say) == "" {
			continue // the grammar prevents this; double-check anyway
		}
		g.Steps = append(g.Steps, GuideStep{Kind: "ui", Go: a.Go, At: a.At, Sel: a.Sel, Text: a.Text, Say: strings.TrimSpace(st.Say), Done: &GuideDone{Click: true}})
	}
	if len(g.Steps) == 0 {
		return nil, errors.New("the offline brain could not build a walkthrough for that")
	}
	if strings.Contains(strings.Join(func() []string {
		var gs []string
		for _, s := range g.Steps {
			gs = append(gs, s.Go)
		}
		return gs
	}(), " "), "{site}") {
		g.Needs = "site"
	}
	return resolveGuide(c, g, site)
}

// ---------- HTTP ----------

func registerGuideRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/jiv/guides", func(w http.ResponseWriter, r *http.Request) {
		var out []map[string]any
		for _, g := range loadGuides() {
			ext := 0
			for _, s := range g.Steps {
				if s.Kind == "external" {
					ext++
				}
			}
			out = append(out, map[string]any{"id": g.ID, "title": g.Title, "title_hi": g.TitleHi, "group": g.Group, "intro": g.Intro, "do": g.Do, "needs": g.Needs, "steps": len(g.Steps), "external": ext})
		}
		writeJSON(w, 200, map[string]any{"guides": out, "anchors": len(guideAnchors())})
	})
	mux.HandleFunc("GET /api/jiv/guides/{id}", func(w http.ResponseWriter, r *http.Request) {
		g := guideByID(r.PathValue("id"))
		if g == nil {
			writeJSON(w, 404, map[string]any{"error": "unknown guide"})
			return
		}
		c := snapshotConfig()
		rg, err := resolveGuide(&c, g, r.URL.Query().Get("site"))
		if errors.Is(err, errNeedSite) {
			var opts []map[string]string
			for _, s := range activeSites(&c) {
				opts = append(opts, map[string]string{"slug": s.Slug, "name": s.Name})
			}
			writeJSON(w, 200, map[string]any{"need_site": true, "sites": opts, "title": g.Title})
			return
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"guide": rg})
	})
	mux.HandleFunc("POST /api/jiv/guides/check", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Check string `json:"check"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		c := snapshotConfig()
		writeJSON(w, 200, runGuideCheck(&c, req.Check))
	})
	mux.HandleFunc("POST /api/jiv/guides/generate", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Q, Site string }
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		c := snapshotConfig()
		if g, _ := matchGuide(req.Q); g != nil {
			rg, err := resolveGuide(&c, g, req.Site)
			if err == nil {
				writeJSON(w, 200, map[string]any{"guide": rg})
				return
			}
		}
		if !brainAvailable(&c) {
			writeJSON(w, 409, map[string]any{"error": "There is no built-in walkthrough for that yet. With the offline brain (Settings > Jiv) Jiv can build one for any question.", "need_brain": true})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
		defer cancel()
		g, err := generateGuide(ctx, &c, req.Q, req.Site)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"guide": g})
	})
}

// guidesForArticle lists guides linked to a knowledge-base article.
func guidesForArticle(id string) []guideRef {
	var out []guideRef
	for _, g := range loadGuides() {
		if g.KB == id {
			out = append(out, guideRef{g.ID, g.Title, g.Do})
		}
	}
	return out
}

var (
	reDefinition = regexp.MustCompile(`^(what|who|why|which|where) (is|are|do|does)\b|\b(kya hai|kya hota|matlab)\b|\bmeaning\b|\bdifference\b`)
	reGuideWords = regexp.MustCompile(`\b(please |can you |could you )?(show me how to|show me how|show me|walk me through|guide me (through|to)?|teach me (how to)?|step by step|walkthrough|how do i|how can i|how to|dikhao|sikhao)\b`)
)

// stripGuideWords removes "show me how to" and similar, leaving the task.
func stripGuideWords(q string) string {
	return strings.TrimSpace(reGuideWords.ReplaceAllString(q, " "))
}
