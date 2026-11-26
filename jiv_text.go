package main

// Text helpers for Jiv's offline brain: tokenizer, light stemming,
// synonyms (English, Hinglish and Devanagari) and fuzzy matching.

import (
	"strings"
	"unicode"
)

var jivStop = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an the is are was were be been am i me my mine we our you your it its this that these those to of in on at for from with by as or and but if then so do does did done can could should would will shall may might must
	how what why when where which who whom whose there here please pls plz hi hello hey thanks thank ok okay just also still yet any some all get got want need like know tell show give let us about into out up than too very much more most
	kaise kya kyun kyon kyu hai hain ho tha thi the ka ki ke ko se me mein mai main mera meri mere apna apni apne kar karo karna karein kare kariye karu hoga hogi raha rahi rahe ye yeh woh wo bhi aur ya to na please batao bataye bata chahiye sakta sakti sakte
	कैसे क्या क्यों है हैं हो था थी के का की को से में मैं मेरा मेरी मेरे अपना करें करो करना कर रहा रही यह वह भी और या तो बताओ बताइए चाहिए`) {
		jivStop[w] = true
	}
}

// jivSyn maps words to one canonical term so "website", "site" and
// "वेबसाइट" all match the same articles.
var jivSyn = map[string]string{
	"dilreet": "rover", "website": "site", "websit": "site", "webpage": "site", "web": "site", "sites": "site", "project": "site", "projects": "site", "app": "site", "वेबसाइट": "site", "साइट": "site", "प्रोजेक्ट": "site",
	"domains": "domain", "domian": "domain", "domin": "domain", "url": "domain", "डोमेन": "domain",
	"ssl": "https", "tls": "https", "certificate": "https", "certif": "https", "cert": "https", "certs": "https", "padlock": "https", "letsencrypt": "https", "secure": "https", "एसएसएल": "https",
	"nameservers": "nameserver", "ns": "nameserver",
	"publish": "deploy", "upload": "deploy", "deployment": "deploy", "redeploy": "deploy", "डिप्लॉय": "deploy", "अपलोड": "deploy",
	"revert": "rollback", "previous": "rollback", "restore": "rollback", "undo": "undo", "रोलबैक": "rollback", "purana": "rollback",
	"broken": "down", "crash": "down", "crashed": "down", "dead": "down", "unreachable": "down", "band": "down", "बंद": "down", "exited": "down", "degraded": "down",
	"fail": "error", "failed": "error", "failure": "error", "problem": "error", "issue": "error", "wrong": "error", "galti": "error", "गलती": "error", "समस्या": "error", "dikkat": "error", "error": "error",
	"vps": "server", "droplet": "server", "instance": "server", "machine": "server", "servers": "server", "सर्वर": "server",
	"chatbot": "jiv", "bot": "jiv", "assistant": "jiv", "helper": "jiv",
	"delete": "delete", "remove": "delete", "hatao": "delete", "हटाएँ": "delete",
	"jode": "connect", "jodna": "connect", "jodu": "connect", "link": "connect", "attach": "connect", "जोड़ें": "connect", "जोड़": "connect",
	"banaye": "create", "banao": "create", "make": "create", "new": "create", "बनाएँ": "create", "नया": "create",
	"kholo": "open", "खोलें": "open",
	"godaddi": "godaddy", "गोडैडी": "godaddy",
	"backups": "backup", "बैकअप": "backup",
	"files": "file", "फ़ाइल": "file", "फाइल": "file", "फ़ाइलें": "file",
	"pass": "password", "पासवर्ड": "password",
	"chal": "work", "chalta": "work", "chalti": "work", "working": "work", "works": "work", "kaam": "work", "काम": "work", "चल": "work",
	"nahi": "not", "nahin": "not", "नहीं": "not", "dont": "not", "doesnt": "not", "isnt": "not", "cant": "not", "wont": "not",
	"khul": "open", "खुल": "open", "opening": "open",
	"model": "brain", "llm": "brain", "llama": "brain", "qwen": "brain", "gguf": "brain", "ब्रेन": "brain",
	"offline": "offline", "ऑफ़लाइन": "offline", "internet": "offline",
	"key": "token", "keys": "token", "tokens": "token",
	"cost": "price", "pricing": "price", "kitne": "price", "kitna": "price", "cheap": "price", "cheapest": "price", "costs": "price",
	"buy":   "create",
	"hindi": "hindi", "हिंदी": "hindi",
	"provider": "hosting", "providers": "hosting", "होस्टिंग": "hosting",
	"memory": "ram",
	"logs":   "log", "लॉग": "log",
	"redirect": "redirects", "redirection": "redirects",
	"ports":  "port",
	"faster": "speed", "slow": "speed",
}

// phrase replacements applied before tokenising.
var jivPhrases = []struct{ from, to string }{
	{"go daddy", "godaddy"}, {"digital ocean", "digitalocean"}, {"lets encrypt", "letsencrypt"}, {"let's encrypt", "letsencrypt"},
	{"roll back", "rollback"}, {"go back", "rollback"}, {"not working", "down notwork"}, {"doesn't work", "down notwork"}, {"does not work", "down notwork"},
	{"isn't working", "down notwork"}, {"won't load", "down"}, {"not loading", "down"}, {"not opening", "down"}, {"nahi chal", "down"}, {"nahi khul", "down"},
	{"चल नहीं", "down"}, {"नहीं चल", "down"}, {"नहीं खुल", "down"}, {"a record", "arecord"}, {"a-record", "arecord"}, {"name server", "nameserver"},
	{"ctrl+k", "ctrl palette"}, {"ctrl k", "ctrl palette"}, {"command palette", "palette"}, {"aws lightsail", "lightsail"}, {"oracle cloud", "oracle"},
	{"node.js", "nodejs"}, {"index.html", "indexhtml"}, {"index html", "indexhtml"}, {"api key", "apitoken"}, {"api token", "apitoken"}, {"access key", "apitoken"},
	{"sslip.io", "sslip"}, {"netlify.toml", "netlify toml"}, {"_redirects", "redirects"}, {"_headers", "headers"}, {"deadline exceeded", "timeout"},
	{"timed out", "timeout"}, {"too many requests", "ratelimit"}, {"rate limit", "ratelimit"}, {"no space left", "diskfull"}, {"disk full", "diskfull"},
	{"host key", "hostkey"}, {"bad gateway", "down 502"}, {"keyboard shortcut", "shortcut"},
}

func jivNormalize(s string) string {
	s = strings.ToLower(s)
	s = strings.NewReplacer("’", "'", "‘", "'", "“", "\"", "”", "\"").Replace(s)
	for _, p := range jivPhrases {
		s = strings.ReplaceAll(s, p.from, p.to)
	}
	return s
}

func isDevanagari(r rune) bool { return r >= 0x0900 && r <= 0x097F }

// jivWords splits text into raw lowercase words (letters, digits, and
// Devanagari including its combining marks).
func jivWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || isDevanagari(r))
	})
}

func jivStem(w string) string {
	if !isASCII(w) || len(w) <= 3 {
		return w
	}
	switch {
	case strings.HasSuffix(w, "ies") && len(w) > 4:
		return w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "ing") && len(w) > 5:
		w = w[:len(w)-3]
		if len(w) > 2 && w[len(w)-1] == w[len(w)-2] {
			w = w[:len(w)-1]
		}
		return w
	case strings.HasSuffix(w, "ed") && len(w) > 4:
		return w[:len(w)-2]
	case strings.HasSuffix(w, "es") && len(w) > 4 && strings.ContainsAny(w[len(w)-3:len(w)-2], "sxzh"):
		return w[:len(w)-2]
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us") && len(w) > 3:
		return w[:len(w)-1]
	}
	return w
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// jivTerms turns text into search terms: normalize, split, drop stop
// words, map synonyms, stem.
func jivTerms(s string) []string {
	var out []string
	for _, w := range jivWords(jivNormalize(s)) {
		if jivStop[w] {
			continue
		}
		if c, ok := jivSyn[w]; ok {
			w = c
		} else {
			w = jivStem(w)
			if c, ok := jivSyn[w]; ok {
				w = c
			}
		}
		if w == "" || jivStop[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

// editDistance is the Damerau-Levenshtein (optimal string alignment)
// distance, capped for speed.
func editDistance(a, b string, max int) int {
	ra, rb := []rune(a), []rune(b)
	if d := len(ra) - len(rb); d > max || -d > max {
		return max + 1
	}
	prev2 := make([]int, len(rb)+1)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		best := cur[0]
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			v := min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				v = min(v, prev2[j-2]+1)
			}
			cur[j] = v
			best = min(best, v)
		}
		if best > max {
			return max + 1
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(rb)]
}

// jivLang guesses the reply language: "hi" for Devanagari or clear
// Hinglish, otherwise "en".
func jivLang(s string) string {
	dev, letters := 0, 0
	for _, r := range s {
		if isDevanagari(r) {
			dev++
		}
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if dev > 0 && dev*3 >= letters {
		return "hi"
	}
	hits := 0
	for _, w := range jivWords(strings.ToLower(s)) {
		switch w {
		case "kaise", "kya", "kyun", "kyon", "kyu", "hai", "nahi", "nahin", "karo", "mera", "meri", "mere", "kar", "raha", "rahi", "batao", "chahiye", "kaun", "kahan", "hoga":
			hits++
		}
	}
	if hits >= 2 {
		return "hi"
	}
	return "en"
}
