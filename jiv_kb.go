package main

// Jiv's built-in knowledge base: embedded articles (jivkb/*.md) searched
// with BM25 entirely on this PC.

import (
	"embed"
	"math"
	"sort"
	"strings"
	"sync"
)

//go:embed jivkb/*.md
var jivKBFS embed.FS

type KBButton struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

type KBArticle struct {
	ID      string     `json:"id"`
	Title   string     `json:"title"`
	Q       []string   `json:"q,omitempty"`
	Tags    []string   `json:"tags,omitempty"`
	Hi      []string   `json:"hi,omitempty"`
	Go      []KBButton `json:"go,omitempty"`
	Do      string     `json:"do,omitempty"`
	Screens []string   `json:"screens,omitempty"`
	Body    string     `json:"body"`
	BodyHi  string     `json:"body_hi,omitempty"`
}

type KBHit struct {
	*KBArticle
	Score float64 `json:"score"`
	Cover float64 `json:"-"` // share of the question (idf-weighted) the article covers
}

type kbIndex struct {
	docs  []*KBArticle
	tf    []map[string]float64
	dl    []float64
	avgdl float64
	df    map[string]int
	vocab []string
	head  []map[string]bool // terms in title, questions and tags
}

var (
	kbOnce sync.Once
	kb     *kbIndex
)

func jivKB() *kbIndex {
	kbOnce.Do(func() {
		var docs []*KBArticle
		ents, _ := jivKBFS.ReadDir("jivkb")
		for _, e := range ents {
			b, err := jivKBFS.ReadFile("jivkb/" + e.Name())
			if err == nil {
				docs = append(docs, parseKB(string(b))...)
			}
		}
		kb = buildKBIndex(docs)
	})
	return kb
}

func splitList(s, sep string) []string {
	var out []string
	for _, p := range strings.Split(s, sep) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseKB(src string) []*KBArticle {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	var out []*KBArticle
	for _, chunk := range strings.Split("\n"+src, "\n=== ")[1:] {
		head, body, _ := strings.Cut(chunk, "\n---\n")
		lines := strings.Split(head, "\n")
		a := &KBArticle{ID: strings.TrimSpace(lines[0])}
		for _, l := range lines[1:] {
			k, v, ok := strings.Cut(l, ":")
			if !ok {
				continue
			}
			v = strings.TrimSpace(v)
			switch strings.TrimSpace(k) {
			case "title":
				a.Title = v
			case "q":
				a.Q = splitList(v, "|")
			case "hi":
				a.Hi = splitList(v, "|")
			case "tags":
				a.Tags = strings.Fields(v)
			case "do":
				a.Do = v
			case "screen":
				a.Screens = strings.Fields(v)
			case "go":
				for _, g := range splitList(v, ";") {
					if lbl, href, ok := strings.Cut(g, "->"); ok {
						a.Go = append(a.Go, KBButton{strings.TrimSpace(lbl), strings.TrimSpace(href)})
					}
				}
			}
		}
		en, hi, _ := strings.Cut(body, "\n@hi\n")
		a.Body, a.BodyHi = strings.TrimSpace(en), strings.TrimSpace(hi)
		out = append(out, a)
	}
	return out
}

// field weights: questions and titles count most.
func kbDocTerms(a *KBArticle) map[string]float64 {
	tf := map[string]float64{}
	add := func(s string, w float64) {
		for _, t := range jivTerms(s) {
			tf[t] += w
		}
	}
	add(a.Title, 3)
	for _, q := range a.Q {
		add(q, 2)
	}
	for _, q := range a.Hi {
		add(q, 2)
	}
	add(strings.Join(a.Tags, " "), 2)
	add(a.Body, 0.5)
	add(a.BodyHi, 0.5)
	return tf
}

func buildKBIndex(docs []*KBArticle) *kbIndex {
	ix := &kbIndex{docs: docs, df: map[string]int{}}
	total := 0.0
	for _, d := range docs {
		tf := kbDocTerms(d)
		hd := map[string]bool{}
		for _, t := range jivTerms(d.Title + " " + strings.Join(d.Q, " ") + " " + strings.Join(d.Hi, " ") + " " + strings.Join(d.Tags, " ")) {
			hd[t] = true
		}
		ix.head = append(ix.head, hd)
		l := 0.0
		for t, n := range tf {
			ix.df[t]++
			l += n
		}
		ix.tf = append(ix.tf, tf)
		ix.dl = append(ix.dl, l)
		total += l
	}
	if len(docs) > 0 {
		ix.avgdl = total / float64(len(docs))
	}
	for t := range ix.df {
		ix.vocab = append(ix.vocab, t)
	}
	sort.Strings(ix.vocab)
	return ix
}

// fuzzy maps an unknown word to the closest known term (typos like
// "godday", "certficate").
func (ix *kbIndex) fuzzy(t string) string {
	if _, ok := ix.df[t]; ok || len([]rune(t)) < 5 {
		return t
	}
	max := 1
	if len([]rune(t)) >= 8 {
		max = 2
	}
	best, bestD, bestDF := t, max+1, 0
	for _, v := range ix.vocab {
		d := editDistance(t, v, max)
		if d < bestD || (d == bestD && ix.df[v] > bestDF) {
			best, bestD, bestDF = v, d, ix.df[v]
		}
	}
	if bestD <= max {
		return best
	}
	return t
}

func (ix *kbIndex) Get(id string) *KBArticle {
	for _, d := range ix.docs {
		if d.ID == id {
			return d
		}
	}
	return nil
}

// Search returns the best articles for a question (BM25, k1=1.2, b=0.75).
// screen (optional) gives a small boost to articles about the current page.
func (ix *kbIndex) Search(q, screen string, n int) []KBHit {
	terms := jivTerms(q)
	seen := map[string]bool{}
	var qs []string
	fuzzed := map[string]bool{}
	for _, t := range terms {
		f := ix.fuzzy(t)
		if f != t {
			fuzzed[f] = true
		}
		if !seen[f] {
			seen[f] = true
			qs = append(qs, f)
		}
	}
	if len(qs) == 0 && screen != "" { // "what can I do here?"
		var hits []KBHit
		for _, d := range ix.ForScreen(screen) {
			hits = append(hits, KBHit{d, 3, 1})
		}
		if n > 0 && len(hits) > n {
			hits = hits[:n]
		}
		return hits
	}
	const k1, b = 1.2, 0.75
	N := float64(len(ix.docs))
	idf := func(t string) float64 {
		df := float64(ix.df[t])
		v := math.Log(1 + (N-df+0.5)/(df+0.5))
		if fuzzed[t] {
			v *= 0.8 // a guessed typo counts a bit less
		}
		return v
	}
	totalIDF := 0.0
	for _, t := range qs {
		if ix.df[t] > 0 {
			totalIDF += idf(t)
		} else {
			totalIDF += math.Log(1 + (N+0.5)/0.5) // unknown word: rare, so it matters
		}
	}
	var hits []KBHit
	for i, d := range ix.docs {
		s, cov, head := 0.0, 0.0, 0.0
		for _, t := range qs {
			f := ix.tf[i][t]
			if f == 0 {
				continue
			}
			w := idf(t)
			s += w * f * (k1 + 1) / (f + k1*(1-b+b*ix.dl[i]/ix.avgdl))
			cov += w
			if ix.head[i][t] {
				head += w
			}
		}
		if s <= 0 {
			continue
		}
		if totalIDF > 0 {
			cov /= totalIDF
			head /= totalIDF
		}
		s *= 1 + 0.6*head // the question matches the article's own title/questions
		for _, sc := range d.Screens {
			if sc == screen {
				s *= 1.15
			}
		}
		hits = append(hits, KBHit{d, s, cov})
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].Score > hits[b].Score })
	if n > 0 && len(hits) > n {
		hits = hits[:n]
	}
	return hits
}

// ForScreen returns the articles that explain a screen.
func (ix *kbIndex) ForScreen(screen string) []*KBArticle {
	var out []*KBArticle
	for _, d := range ix.docs {
		for _, s := range d.Screens {
			if s == screen {
				out = append(out, d)
			}
		}
	}
	return out
}

// confident reports whether the top hit is a good answer on its own.
func kbConfident(h []KBHit) bool {
	if len(h) == 0 {
		return false
	}
	if h[0].Score < 2.5 || h[0].Cover < 0.5 {
		return false
	}
	return len(h) == 1 || h[0].Score >= h[1].Score*1.1 || h[0].Score > 6
}
