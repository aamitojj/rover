package main

// Jiv's ask pipeline (POST /api/jiv/ask, NDJSON stream):
//   1. intent: requests become action cards; problems run a read-only diagnosis
//   2. knowledge base: BM25 over the built-in articles (always offline)
//   3. a model, when available: the offline brain (llama-server sidecar),
//      a local AI app, or a cloud AI the user chose - with the KB articles
//      and app state as context (RAG) and native tool calls.
// Without any model Jiv still answers from the knowledge base.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type askReq struct {
	Q       string    `json:"q"`
	Screen  string    `json:"screen"`
	Site    string    `json:"site"` // project open in the UI
	History []ChatMsg `json:"history"`
	Target  string    `json:"target"` // "" / auto | builtin | brain | local:<id> | <provider>
	Model   string    `json:"model"`
	Lang    string    `json:"lang"`
	About   string    `json:"about"` // error text from "Why did it fail?"
}

type articleRef struct {
	ID     string     `json:"id"`
	Title  string     `json:"title"`
	Go     []KBButton `json:"go,omitempty"`
	Guides []guideRef `json:"guides,omitempty"` // 0.8: "Show me how" walkthroughs
}

func refOf(a *KBArticle) articleRef { return articleRef{a.ID, a.Title, a.Go, guidesForArticle(a.ID)} }

// pickTarget decides who writes the answer.
func pickTarget(ctx context.Context, c *Config, req askReq) (*aiTarget, string, error) {
	target := req.Target
	if target == "" || target == "auto" {
		switch {
		case brainAvailable(c):
			target = "brain"
		case c.AIDefault != "" && strings.HasPrefix(c.AIDefault, "local:"):
			t, m, _ := strings.Cut(c.AIDefault, "|")
			target, req.Model = t, m
		default:
			return nil, "builtin", nil
		}
	}
	switch target {
	case "builtin":
		return nil, "builtin", nil
	case "brain":
		if !brainAvailable(c) {
			return nil, "builtin", nil
		}
		base, key, err := brainSidecar.Ensure(ctx)
		if err != nil {
			return nil, "builtin", err
		}
		_, _, m, _ := brainPaths(c)
		return &aiTarget{Label: "Jiv offline brain (" + m.Short + ")", Company: "this PC", Adapter: &openAIAdapter{name: "Offline brain", base: base, key: key}, Model: m.ID, Native: true}, "brain", nil
	}
	t, err := resolveTarget(ctx, target, req.Model)
	if err != nil {
		return nil, "builtin", err
	}
	if t.Cloud && t.ConnID != "" {
		if cn := c.AIConns[t.ConnID]; cn == nil || !cn.Consent {
			return t, "builtin", &AIError{Kind: "consent", Message: "consent needed"}
		}
	}
	mode := "local"
	if t.Cloud {
		mode = "cloud"
	}
	return t, mode, nil
}

const jivSystem = `You are Jiv, the helper inside Rover, a Windows app that publishes websites to the user's own server (VPS) with the hosting engine Coolify. Users are often not technical.
Rules: answer in short, plain, friendly language; use numbered steps for how-tos; only describe buttons and screens that appear in the KNOWLEDGE below; if unsure, say so and point to the right screen.
Never ask for passwords, tokens or API keys in chat - tell the user where in Rover to enter them. Rover never needs website passwords for hosting providers.
When the user asks you to DO something, call the matching tool. Nothing runs until the user clicks the confirm card, so never say it already happened.
Projects are named by their slug (see APP STATE).`

func buildJivSystem(c *Config, hits []KBHit, diag *Diagnosis, lang, about string, cloud bool) string {
	var b strings.Builder
	b.WriteString(jivSystem)
	if lang == "hi" {
		b.WriteString("\nReply in simple Hindi (Devanagari), keeping button names in English.")
	}
	b.WriteString("\n\nKNOWLEDGE:\n")
	for i, h := range hits {
		if i >= 3 {
			break
		}
		fmt.Fprintf(&b, "## %s\n%s\n\n", h.Title, clip(h.Body, 1400))
	}
	ctx := appContext() + hostingContext(c)
	if diag != nil {
		ctx += "\nDIAGNOSIS just run by Rover (" + diag.Summary + "):"
		for _, f := range diag.Findings {
			ctx += fmt.Sprintf("\n- [%s] %s: %s", f.Level, f.Title, f.Text)
		}
	}
	if about != "" {
		ctx += "\nThe user is asking about this message from the app:\n" + clip(about, 1500)
	}
	b.WriteString("APP STATE:\n" + redact(ctx, cloud))
	return b.String()
}

// hostingContext lists accounts and servers by nickname only - never
// credentials (they are not even decrypted here).
func hostingContext(c *Config) string {
	var b strings.Builder
	if len(c.Servers) > 0 {
		b.WriteString("\nServers:")
		for _, p := range c.Servers {
			fmt.Fprintf(&b, "\n- %s (%s)%s", p.Label, p.ServerIP, map[bool]string{true: " [active]", false: ""}[p.ID == c.ActiveServer])
		}
	}
	if len(c.HostingAccounts) > 0 {
		b.WriteString("\nHosting accounts:")
		for _, a := range c.HostingAccounts {
			p, _ := hostingProviderByID(a.Provider)
			fmt.Fprintf(&b, "\n- %q at %s, %d servers, status %s", a.Nickname, p.Name, len(a.Servers), firstNonEmpty(a.Status, "unknown"))
		}
	}
	if !brainAvailable(c) {
		b.WriteString("\nOffline brain: not installed (Settings > Jiv).")
	}
	return b.String()
}

var jivStrings = map[string]map[string]string{
	"confirm":   {"en": "Sure. Please check and confirm:", "hi": "ठीक है। कृपया जाँचकर पुष्टि करें:"},
	"opening":   {"en": "Opening it for you.", "hi": "खोल रहा हूँ।"},
	"which":     {"en": "Which project do you mean?", "hi": "कौन-सा प्रोजेक्ट?"},
	"checking":  {"en": "Let me check your setup…", "hi": "मैं जाँच कर रहा हूँ…"},
	"found":     {"en": "Here's what I found in the help:", "hi": "मदद में यह मिला:"},
	"related":   {"en": "Related", "hi": "संबंधित"},
	"suggest":   {"en": "I can also do it for you:", "hi": "मैं यह आपके लिए भी कर सकता हूँ:"},
	"enonly":    {"en": "", "hi": "(यह जवाब अभी अंग्रेज़ी में है।)"},
	"noanswer":  {"en": "I don't have a built-in answer for that yet. Try different words, pick a topic below, or download the offline brain in Settings > Jiv for free-form questions (it runs on this PC).", "hi": "इसका जवाब मेरे पास अभी नहीं है। दूसरे शब्दों में पूछें, नीचे से कोई विषय चुनें, या Settings > Jiv में ऑफ़लाइन ब्रेन डाउनलोड करें।"},
	"diagfail":  {"en": "I couldn't run the check: ", "hi": "जाँच नहीं हो पाई: "},
	"brainwait": {"en": "The offline brain is still loading, so here is the built-in answer.", "hi": "ऑफ़लाइन ब्रेन अभी लोड हो रहा है, इसलिए यह बिल्ट-इन जवाब है।"},
}

func jt(key, lang string) string {
	if v := jivStrings[key][lang]; v != "" || lang == "hi" && key == "enonly" {
		return v
	}
	return jivStrings[key]["en"]
}

func apiJivAsk(w http.ResponseWriter, r *http.Request) {
	var req askReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		writeErr(w, err)
		return
	}
	req.Q = strings.TrimSpace(req.Q)
	if req.Q == "" {
		writeErr(w, errors.New("type a question"))
		return
	}
	c := snapshotConfig()
	lang := req.Lang
	if lang == "" || lang == "auto" {
		lang = jivSettings(&c).Lang
	}
	if lang == "" || lang == "auto" {
		lang = jivLang(req.Q)
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	fl, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	send := func(v map[string]any) {
		_ = enc.Encode(v)
		if fl != nil {
			fl.Flush()
		}
	}
	defer send(map[string]any{"t": "done"})

	// The project open in the UI counts as "this site".
	q := req.Q
	in := detectIntent(&c, q)
	if in.NeedSite && req.Site != "" && findSite(&c, req.Site) != nil {
		in.Call.Args["site"], in.NeedSite = req.Site, false
	}
	hits := jivKB().Search(q+" "+req.About, req.Screen, 5)

	// Decide the writer early so the UI can show it.
	tctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	t, mode, terr := pickTarget(tctx, &c, req)
	var ae *AIError
	if errors.As(terr, &ae) && ae.Kind == "consent" && t != nil {
		send(map[string]any{"t": "start", "mode": "builtin", "label": "Jiv (built-in, offline)", "lang": lang})
		send(map[string]any{"t": "error", "kind": "consent", "conn": t.ConnID, "company": t.Company, "v": "Allow " + t.Company + " first, or pick the built-in Jiv."})
		return
	}
	label := "Jiv (built-in, offline)"
	if t != nil && terr == nil {
		label = t.Label
	}
	send(map[string]any{"t": "start", "mode": mode, "label": label, "lang": lang, "model": func() string {
		if t != nil {
			return t.Model
		}
		return ""
	}()})
	if terr != nil && mode == "builtin" && req.Target == "brain" {
		send(map[string]any{"t": "note", "v": jt("brainwait", lang)})
	}

	// 0. Walkthroughs ("show me how") and multi-step plans ("do it for me").
	if jivAsk08(r.Context(), send, &c, req, in, lang) {
		return
	}

	// 1. Actions and diagnosis.
	var diag *Diagnosis
	if in.Call != nil && (!in.Question || in.Diagnose) {
		if in.NeedSite && len(c.Sites) > 1 {
			var opts []ActionCard
			for i, s := range c.Sites {
				if i >= 8 || s.Archived {
					continue
				}
				args := map[string]any{}
				for k, v := range in.Call.Args {
					args[k] = v
				}
				args["site"] = s.Slug
				if card, err := proposeAction(ToolCall{Name: in.Call.Name, Args: args}); err == nil {
					card.Label = s.Name
					opts = append(opts, *card)
				}
			}
			send(map[string]any{"t": "text", "v": jt("which", lang)})
			send(map[string]any{"t": "choices", "options": opts})
			return
		}
		if in.NeedSite && len(c.Sites) == 0 {
			send(map[string]any{"t": "text", "v": "You don't have any projects yet. Add one first:"})
			card, _ := proposeAction(ToolCall{Name: "open_page", Args: map[string]any{"page": "new-project"}})
			send(map[string]any{"t": "action", "card": card})
			return
		}
		card, err := proposeAction(*in.Call)
		if err != nil {
			send(map[string]any{"t": "text", "v": "I couldn't do that: " + err.Error() + "."})
			return
		}
		if card.Safe && !card.Client && strings.Contains("diagnose check_dns run_health explain_logs", card.Name) {
			send(map[string]any{"t": "status", "v": jt("checking", lang)})
			code, res := runAction(card.ID, false)
			if code != 200 {
				send(map[string]any{"t": "text", "v": jt("diagfail", lang) + fmt.Sprint(res["error"])})
				if sw, ok := res["switch_server"]; ok {
					if c2, err := proposeAction(ToolCall{Name: "switch_server", Args: map[string]any{"server": sw}}); err == nil {
						send(map[string]any{"t": "action", "card": c2})
					}
				}
				return
			}
			if d, ok := res["diagnosis"].(Diagnosis); ok {
				diag = &d
				send(map[string]any{"t": "diag", "diagnosis": proposeFixes(d)})
			}
			if t == nil {
				send(map[string]any{"t": "text", "v": diagText(diag, lang)})
				if diag != nil && len(diag.Findings) > 0 && diag.Findings[0].KB != "" {
					if a := jivKB().Get(diag.Findings[0].KB); a != nil {
						send(map[string]any{"t": "sources", "articles": []articleRef{refOf(a)}})
					}
				}
				return
			}
			// fall through: the model explains the diagnosis in its own words
		} else {
			if card.Client && card.Safe {
				send(map[string]any{"t": "text", "v": jt("opening", lang)})
			} else if !card.Safe {
				send(map[string]any{"t": "text", "v": jt("confirm", lang)})
			}
			send(map[string]any{"t": "action", "card": card})
			return
		}
	}

	// 2. Built-in answer.
	if t == nil {
		builtinAnswer(send, hits, lang, in)
		return
	}

	// 3. Model answer with RAG + tools.
	msgs := append([]ChatMsg{}, req.History...)
	if len(msgs) > 12 {
		msgs = msgs[len(msgs)-12:]
	}
	msgs = append(msgs, ChatMsg{Role: "user", Content: q})
	if t.Cloud {
		for i := range msgs {
			msgs[i].Content = redact(msgs[i].Content, false)
		}
	}
	creq := ChatRequest{Model: t.Model, System: buildJivSystem(&c, hits, diag, lang, req.About, t.Cloud), Messages: msgs}
	if t.Native {
		creq.Tools = aiTools
	} else {
		var names []string
		for _, a := range aiTools {
			names = append(names, a.Name+" ("+a.Description+")")
		}
		creq.System += fmt.Sprintf(aiTextProtocol, strings.Join(names, "; "))
	}
	var full strings.Builder
	var calls []ToolCall
	err := t.Adapter.Stream(tctx, creq, func(ev StreamEvent) {
		if ev.Text != "" {
			full.WriteString(ev.Text)
			send(map[string]any{"t": "text", "v": ev.Text})
		}
		if ev.Tool != nil {
			calls = append(calls, *ev.Tool)
		}
	})
	if err != nil {
		if full.Len() == 0 { // fall back to the built-in answer
			if !errors.As(err, &ae) {
				ae = mapAIError(t.Label, 0, nil, err)
			}
			send(map[string]any{"t": "note", "v": ae.Message + " Here is the built-in answer instead."})
			if diag != nil {
				send(map[string]any{"t": "text", "v": diagText(diag, lang)})
				return
			}
			builtinAnswer(send, hits, lang, in)
			return
		}
		send(map[string]any{"t": "error", "v": err.Error()})
	}
	calls = append(calls, parseTextActions(full.String())...)
	seen := map[string]bool{}
	for _, tc := range calls {
		b, _ := json.Marshal(tc)
		if seen[string(b)] {
			continue
		}
		seen[string(b)] = true
		card, err := proposeAction(tc)
		if err != nil {
			continue // unknown tools/projects are dropped, never executed
		}
		send(map[string]any{"t": "action", "card": card})
	}
	if diag == nil && len(hits) > 0 && hits[0].Score > 2 {
		var refs []articleRef
		for i, h := range hits {
			if i < 2 {
				refs = append(refs, refOf(h.KBArticle))
			}
		}
		send(map[string]any{"t": "sources", "articles": refs})
	}
}

// proposeFixes turns finding fixes into confirmable cards.
func proposeFixes(d Diagnosis) map[string]any {
	type fv struct {
		Finding
		Card *ActionCard `json:"card,omitempty"`
	}
	var out []fv
	for _, f := range d.Findings {
		v := fv{Finding: f}
		if f.Fix != nil {
			if card, err := proposeAction(*f.Fix); err == nil {
				if f.FixLbl != "" {
					card.Label = f.FixLbl
				}
				v.Card = card
			}
		}
		out = append(out, v)
	}
	return map[string]any{"site": d.Site, "name": d.Name, "summary": d.Summary, "findings": out}
}

func diagText(d *Diagnosis, lang string) string {
	if d == nil {
		return ""
	}
	if lang == "hi" {
		bad := 0
		for _, f := range d.Findings {
			if f.Level == "fail" || f.Level == "warn" {
				bad++
			}
		}
		if bad == 0 {
			return "मैंने जाँच की - कोई समस्या नहीं मिली। " + d.Summary
		}
		return fmt.Sprintf("मैंने जाँच की और %d बात(ें) ठीक करनी हैं। नीचे हर एक का कारण और उपाय है। (%s)", bad, d.Summary)
	}
	return d.Summary
}

func builtinAnswer(send func(map[string]any), hits []KBHit, lang string, in Intent) {
	if len(hits) == 0 || hits[0].Score < 1.2 {
		send(map[string]any{"t": "text", "v": jt("noanswer", lang)})
		send(map[string]any{"t": "topics", "articles": topicRefs()})
		return
	}
	if !kbConfident(hits) {
		send(map[string]any{"t": "text", "v": jt("found", lang)})
		var refs []articleRef
		for i, h := range hits {
			if i < 4 {
				refs = append(refs, refOf(h.KBArticle))
			}
		}
		send(map[string]any{"t": "topics", "articles": refs})
		return
	}
	a := hits[0].KBArticle
	body := a.Body
	if lang == "hi" {
		if a.BodyHi != "" {
			body = a.BodyHi
		} else if n := jt("enonly", lang); n != "" {
			body = n + "\n\n" + body
		}
	}
	send(map[string]any{"t": "text", "v": "**" + a.Title + "**\n\n" + body})
	send(map[string]any{"t": "sources", "articles": []articleRef{refOf(a)}, "primary": true})
	if in.Call != nil {
		if card, err := proposeAction(*in.Call); err == nil && !in.NeedSite {
			send(map[string]any{"t": "text", "v": "\n\n" + jt("suggest", lang)})
			send(map[string]any{"t": "action", "card": card, "suggested": true})
		}
	}
	var rel []articleRef
	for _, h := range hits[1:] {
		if len(rel) < 3 && h.Score > hits[0].Score*0.45 {
			rel = append(rel, refOf(h.KBArticle))
		}
	}
	if len(rel) > 0 {
		send(map[string]any{"t": "related", "articles": rel})
	}
}

func topicRefs() []articleRef {
	var out []articleRef
	for _, id := range []string{"first-steps", "new-project", "add-domain", "site-down", "hosting-accounts", "rollback", "offline-brain", "jiv-actions"} {
		if a := jivKB().Get(id); a != nil {
			out = append(out, refOf(a))
		}
	}
	return out
}

// suggestions are context-aware starter questions per screen.
func jivSuggestions(c *Config, screen, site string) []string {
	var out []string
	add := func(s ...string) { out = append(out, s...) }
	for _, s := range c.Sites {
		if s.Archived || !onActive(c, s) {
			continue
		}
		if d := lastDeployOf(c, s); d != nil && d.Status == "failed" {
			add("Why did " + s.Slug + " fail?")
			break
		}
	}
	for _, s := range c.Sites {
		if s.Domain != "" && !s.DomainLive {
			add("Why does " + s.Domain + " not work yet?")
			break
		}
	}
	switch screen {
	case "hosting":
		add("Which providers can Rover control?", "How much does a server cost?", "Connect my Hetzner account")
	case "domain", "domains":
		add("How do I connect a GoDaddy domain?", "Will my email keep working?", "When does HTTPS start?")
	case "new":
		add("This project must be built first - what do I do?", "Can I start from a template?", "Deploy from GitHub")
	case "deploys":
		add("How do I roll back?", "Why did my deploy fail?")
	case "settings", "jiv-settings", "ai":
		add("What is the offline brain?", "Is my password stored?")
	case "testing":
		add("How do I test before publishing?", "How do I add a redirect?")
	case "project", "files", "logs", "project-settings":
		if site != "" {
			add("Is "+site+" OK?", "Explain the logs of "+site, "Roll back "+site)
		}
	}
	if c.ServerIP == "" {
		add("How do I start?", "Connect a hosting account")
	}
	if len(out) < 4 {
		add("What needs my attention?", "What can you do?", "How do I connect my domain?", "Meri site band kyun hai?")
	}
	seen := map[string]bool{}
	var uniq []string
	for _, s := range out {
		if !seen[s] && len(uniq) < 5 {
			seen[s] = true
			uniq = append(uniq, s)
		}
	}
	return uniq
}

// ---------- chat history (local file) ----------

func historyPath() (string, error) {
	d, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "jiv-history.json"), nil
}

func registerJivRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/jiv/ask", apiJivAsk)
	mux.HandleFunc("GET /api/jiv/search", func(w http.ResponseWriter, r *http.Request) {
		hits := jivKB().Search(r.URL.Query().Get("q"), r.URL.Query().Get("screen"), 6)
		var out []map[string]any
		for _, h := range hits {
			out = append(out, map[string]any{"id": h.ID, "title": h.Title, "go": h.Go, "score": h.Score, "snippet": clip(plainMD(h.Body), 140)})
		}
		writeJSON(w, 200, map[string]any{"results": out})
	})
	mux.HandleFunc("GET /api/jiv/article/{id}", func(w http.ResponseWriter, r *http.Request) {
		a := jivKB().Get(r.PathValue("id"))
		if a == nil {
			writeJSON(w, 404, map[string]any{"error": "not found"})
			return
		}
		writeJSON(w, 200, a)
	})
	mux.HandleFunc("GET /api/jiv/suggest", func(w http.ResponseWriter, r *http.Request) {
		c := snapshotConfig()
		writeJSON(w, 200, map[string]any{"questions": jivSuggestions(&c, r.URL.Query().Get("screen"), r.URL.Query().Get("site"))})
	})
	mux.HandleFunc("POST /api/jiv/propose", func(w http.ResponseWriter, r *http.Request) {
		var tc ToolCall
		if err := readJSON(r, &tc); err != nil {
			writeErr(w, err)
			return
		}
		card, err := proposeAction(tc)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, card)
	})
	mux.HandleFunc("POST /api/jiv/actions/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Confirmed bool `json:"confirmed"`
		}
		_ = readJSON(r, &req)
		code, out := runAction(r.PathValue("id"), req.Confirmed)
		writeJSON(w, code, out)
	})
	mux.HandleFunc("GET /api/jiv/history", func(w http.ResponseWriter, r *http.Request) {
		p, err := historyPath()
		if err != nil {
			writeErr(w, err)
			return
		}
		b, err := os.ReadFile(p)
		if err != nil {
			writeJSON(w, 200, map[string]any{"messages": []any{}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("PUT /api/jiv/history", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []json.RawMessage `json:"messages"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&req); err != nil {
			writeErr(w, err)
			return
		}
		if len(req.Messages) > 200 {
			req.Messages = req.Messages[len(req.Messages)-200:]
		}
		p, err := historyPath()
		if err == nil {
			b, _ := json.Marshal(req)
			err = os.WriteFile(p, b, 0o600)
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/jiv/brain", func(w http.ResponseWriter, r *http.Request) {
		c := snapshotConfig()
		hw := hardware(r.URL.Query().Get("refresh") == "1")
		br := recommendFor(hw)
		rec, ram := br.Model, hw.RAM
		exe, model, m, ok := brainPaths(&c)
		js := jivSettings(&c)
		writeJSON(w, 200, map[string]any{
			"hardware": hw, "advice": br, "pack": packStatus(),
			"models": brainModels, "recommended": rec, "ram_total": ram, "cpus": cpuCount(),
			"installed": ok, "model": m, "model_path": model, "runtime_path": exe, "settings": js,
			"status": brainSidecar.Status(), "runtime": brainRuntimes["cpu"], "runtime_gpu": brainRuntimes["vulkan"], "build": llamaBuild,
		})
	})
	mux.HandleFunc("POST /api/jiv/brain/install", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
			GPU   bool   `json:"gpu"`
		}
		_ = readJSON(r, &req)
		if _, ok := brainModelByID(req.Model); !ok {
			writeErr(w, errors.New("choose Standard or Light"))
			return
		}
		j := newJob("brain-install", func(j *Job) error { return installBrainJob(j, req.Model, req.GPU) })
		writeJSON(w, 200, map[string]any{"job": j.ID})
	})
	mux.HandleFunc("POST /api/jiv/brain/import", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model   string `json:"model_path"`
			Runtime string `json:"runtime_path"`
		}
		_ = readJSON(r, &req)
		id, err := importBrain(req.Model, req.Runtime)
		if err != nil {
			writeErr(w, err)
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			_, _, _ = brainSidecar.Ensure(ctx)
		}()
		writeJSON(w, 200, map[string]any{"ok": true, "model": id})
	})
	mux.HandleFunc("POST /api/jiv/brain/start", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
		defer cancel()
		if _, _, err := brainSidecar.Ensure(ctx); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "status": brainSidecar.Status()})
	})
	mux.HandleFunc("POST /api/jiv/brain/stop", func(w http.ResponseWriter, r *http.Request) {
		brainSidecar.Stop()
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("DELETE /api/jiv/brain", func(w http.ResponseWriter, r *http.Request) {
		if err := removeBrain(); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/jiv/settings", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Lang     *string `json:"lang"`
			BrainOff *bool   `json:"brain_off"`
			NoAuto   *bool   `json:"no_autostart"`
			NoNudges *bool   `json:"no_nudges"`
			IdleMin  *int    `json:"idle_min"`
			Mirror   *string `json:"mirror"`
			GPU      *bool   `json:"gpu"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		if req.Mirror != nil && *req.Mirror != "" && !strings.HasPrefix(*req.Mirror, "https://") {
			writeErr(w, errors.New("the mirror address must start with https://"))
			return
		}
		_ = withConfig(func(c *Config) error {
			if c.Jiv == nil {
				c.Jiv = defaultJivSettings()
			}
			if req.Lang != nil && (*req.Lang == "auto" || *req.Lang == "en" || *req.Lang == "hi") {
				c.Jiv.Lang = *req.Lang
			}
			if req.BrainOff != nil {
				c.Jiv.BrainOff = *req.BrainOff
			}
			if req.NoAuto != nil {
				c.Jiv.NoAuto = *req.NoAuto
			}
			if req.NoNudges != nil {
				c.Jiv.NoNudges = *req.NoNudges
			}
			if req.IdleMin != nil && *req.IdleMin >= 1 && *req.IdleMin <= 1440 {
				c.Jiv.IdleMin = *req.IdleMin
			}
			if req.Mirror != nil {
				c.Jiv.Mirror = strings.TrimSpace(*req.Mirror)
			}
			if req.GPU != nil {
				c.Jiv.GPU = *req.GPU
			}
			return nil
		})
		if (req.BrainOff != nil && *req.BrainOff) || req.GPU != nil {
			brainSidecar.Stop()
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
}

var reMDMarks = regexp.MustCompile("[*`#_]+|\\[([^\\]]*)\\]\\([^)]*\\)")

// plainMD turns a markdown body into one line of plain text (for previews).
func plainMD(s string) string {
	s = reMDMarks.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, "[") {
			return m[1:strings.Index(m, "]")]
		}
		return ""
	})
	return strings.Join(strings.Fields(s), " ")
}
