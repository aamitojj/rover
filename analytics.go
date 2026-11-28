package main

// Rover 0.9 first-party analytics: a tiny beacon and/or nginx access-log
// parsing. No cookies. Estimated uniques are a daily salted hash of
// IP+UA (the raw IP is not stored). Country comes from Cloudflare's
// CF-IPCountry header when present, or from an optional MaxMind-style
// MMDB the user provides (Rover does not bundle GeoLite: the licence
// requires a personal download). Optional GA4 / Search Console / AdSense
// reports are fetched through the Integration Centre.

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AnalyticsSettings struct {
	Enabled       bool   `json:"enabled"`
	RetentionDays int    `json:"retention_days,omitempty"`
	BotFilter     bool   `json:"bot_filter"`
	MMDBPath      string `json:"mmdb_path,omitempty"` // optional, user-provided
	Salt          string `json:"salt,omitempty"`      // protected; rotated per day in the file name, not with the event
}

type analyticsEvent struct {
	Day      string `json:"day"`
	Path     string `json:"path"`
	Referrer string `json:"referrer,omitempty"`
	Country  string `json:"country,omitempty"`
	Region   string `json:"region,omitempty"`
	City     string `json:"city,omitempty"`
	Device   string `json:"device,omitempty"`
	Browser  string `json:"browser,omitempty"`
	Unique   string `json:"unique,omitempty"` // daily salted hash, not reversible to an IP
	Bytes    int64  `json:"bytes,omitempty"`
	Status   int    `json:"status,omitempty"`
	EngageMs int    `json:"engage_ms,omitempty"`
	Bot      bool   `json:"bot,omitempty"`
	Site     string `json:"site"`
}

var botUA = regexp.MustCompile(`(?i)bot|crawl|spider|slurp|bingpreview|facebookexternal|pingdom|uptimerobot|headless|wget|curl|python-requests|go-http-client`)

func analyticsDir(site string) string {
	d, err := configDir()
	if err != nil || d == "" {
		d = docsAppDir()
	}
	return filepath.Join(d, "analytics", site)
}

func dailySalt(day string) string {
	c := snapshotConfig()
	base := c.Analytics
	secretSalt := ""
	if base != nil {
		secretSalt = secret(base.Salt)
	}
	if secretSalt == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		secretSalt = hex.EncodeToString(b)
		enc, err := protectString(secretSalt)
		if err == nil {
			_ = withConfig(func(c *Config) error {
				if c.Analytics == nil {
					c.Analytics = &AnalyticsSettings{Enabled: true, RetentionDays: 90, BotFilter: true}
				}
				if c.Analytics.Salt == "" {
					c.Analytics.Salt = enc
				}
				return nil
			})
		}
	}
	sum := sha256.Sum256([]byte(secretSalt + "|" + day))
	return hex.EncodeToString(sum[:])
}

func uniqueHash(day, ip, ua string) string {
	sum := sha256.Sum256([]byte(dailySalt(day) + "|" + ip + "|" + ua))
	return hex.EncodeToString(sum[:8])
}

func classifyDevice(ua string) (device, browser string) {
	u := strings.ToLower(ua)
	switch {
	case strings.Contains(u, "ipad") || strings.Contains(u, "tablet"):
		device = "tablet"
	case strings.Contains(u, "mobi") || strings.Contains(u, "android"):
		device = "mobile"
	default:
		device = "desktop"
	}
	switch {
	case strings.Contains(u, "edg/"):
		browser = "Edge"
	case strings.Contains(u, "chrome") && !strings.Contains(u, "chromium"):
		browser = "Chrome"
	case strings.Contains(u, "firefox"):
		browser = "Firefox"
	case strings.Contains(u, "safari"):
		browser = "Safari"
	case strings.Contains(u, "opr/") || strings.Contains(u, "opera"):
		browser = "Opera"
	default:
		browser = "Other"
	}
	return
}

func isBot(ua string) bool { return botUA.MatchString(ua) }

func countryOf(r *http.Request) string {
	if c := r.Header.Get("CF-IPCountry"); c != "" && c != "XX" {
		return strings.ToUpper(c)
	}
	if c := r.Header.Get("X-Country"); c != "" {
		return strings.ToUpper(c)
	}
	return ""
}

func clientIP(r *http.Request) string {
	if x := r.Header.Get("CF-Connecting-IP"); x != "" {
		return x
	}
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.TrimSpace(strings.Split(x, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

var analyticsMu sync.Mutex

func recordEvent(ev analyticsEvent) error {
	if ev.Site == "" || ev.Day == "" {
		return fmt.Errorf("incomplete event")
	}
	dir := analyticsDir(ev.Site)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, ev.Day+".jsonl")
	b, _ := json.Marshal(ev)
	analyticsMu.Lock()
	defer analyticsMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func pruneAnalytics(site string, keepDays int) {
	if keepDays <= 0 {
		keepDays = 90
	}
	cut := time.Now().UTC().AddDate(0, 0, -keepDays).Format("2006-01-02")
	ents, err := os.ReadDir(analyticsDir(site))
	if err != nil {
		return
	}
	for _, e := range ents {
		name := strings.TrimSuffix(e.Name(), ".jsonl")
		if name < cut {
			_ = os.Remove(filepath.Join(analyticsDir(site), e.Name()))
		}
	}
}

// roverBeaconJS is the first-party script served at /__rover/r.js (this PC)
// and installed on nginx sites that enable Rover Analytics.
const roverBeaconJS = `(function(){var d=document,l=location,p=l.pathname+l.search,r=d.referrer||"",s=(screen.width||0)+"x"+(screen.height||0),e=0;try{var t=performance&&performance.timing;if(t&&t.navigationStart)e=Math.max(0,(t.domContentLoadedEventEnd||Date.now())-t.navigationStart);}catch(x){}var u="/__rover/e?p="+encodeURIComponent(p)+"&r="+encodeURIComponent(r)+"&s="+encodeURIComponent(s)+"&g="+e;try{if(navigator.sendBeacon){navigator.sendBeacon(u);return;}}catch(x){}var i=new Image();i.referrerPolicy="no-referrer";i.src=u;})();`

func registerAnalyticsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /__rover/r.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		io.WriteString(w, roverBeaconJS)
	})
	mux.HandleFunc("GET /__rover/e", handleBeacon)
	mux.HandleFunc("POST /__rover/e", handleBeacon)
	mux.HandleFunc("GET /api/analytics/{slug}", apiAnalyticsSummary)
	mux.HandleFunc("GET /api/analytics/{slug}/google", apiAnalyticsGoogle)
	mux.HandleFunc("POST /api/analytics/{slug}/ingest-log", apiIngestLog)
}

func handleBeacon(w http.ResponseWriter, r *http.Request) {
	c := snapshotConfig()
	if c.Analytics != nil && !c.Analytics.Enabled {
		w.WriteHeader(204)
		return
	}
	site := r.URL.Query().Get("site")
	if site == "" {
		// This-PC hosting: the site is identified by Host.
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		for _, s := range c.Sites {
			if s.Domain == host || strings.Contains(s.InstantURL, host) || s.Slug+"." == host {
				site = s.Slug
				break
			}
		}
		if site == "" && len(c.Sites) == 1 {
			site = c.Sites[0].Slug
		}
	}
	if site == "" || findSite(&c, site) == nil {
		w.WriteHeader(204)
		return
	}
	ua := r.UserAgent()
	day := time.Now().UTC().Format("2006-01-02")
	ip := clientIP(r)
	dev, br := classifyDevice(ua)
	eng, _ := strconv.Atoi(r.URL.Query().Get("g"))
	ev := analyticsEvent{
		Day: day, Site: site, Path: firstNonEmpty(r.URL.Query().Get("p"), "/"),
		Referrer: stripRef(r.URL.Query().Get("r")), Country: countryOf(r),
		Device: dev, Browser: br, Unique: uniqueHash(day, ip, ua),
		EngageMs: eng, Bot: isBot(ua), Status: 200,
	}
	if c.Analytics != nil && c.Analytics.BotFilter && ev.Bot {
		w.WriteHeader(204)
		return
	}
	_ = recordEvent(ev)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(204)
}

func stripRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	// Keep host+path, drop query (may contain tokens).
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		ref = ref[:i]
	}
	return ref
}

func apiAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	c := snapshotConfig()
	if findSite(&c, slug) == nil {
		writeErr(w, fmt.Errorf("unknown project"))
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 365 {
		days = 28
	}
	writeJSON(w, 200, analyticsSummary(slug, days, c.Analytics != nil && c.Analytics.BotFilter))
}

type analyticsSummaryView struct {
	Pageviews  int              `json:"pageviews"`
	Uniques    int              `json:"uniques"`
	Visitors   int              `json:"visitors"` // same as uniques; alias for the UI
	Bandwidth  int64            `json:"bandwidth"`
	Errors     int              `json:"errors"`
	Bots       int              `json:"bots_filtered"`
	Countries  []analyticsCount `json:"countries"`
	Regions    []analyticsCount `json:"regions"`
	Cities     []analyticsCount `json:"cities"`
	Devices    []analyticsCount `json:"devices"`
	Browsers   []analyticsCount `json:"browsers"`
	Referrers  []analyticsCount `json:"referrers"`
	Pages      []analyticsCount `json:"pages"`
	Days       []analyticsCount `json:"days"`
	Engagement int              `json:"engagement_ms_avg"`
	GeoNote    string           `json:"geo_note"`
	PeriodDays int              `json:"period_days"`
}

type analyticsCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Bytes int64  `json:"bytes,omitempty"`
}

func analyticsSummary(site string, days int, dropBots bool) analyticsSummaryView {
	out := analyticsSummaryView{GeoNote: "Country (and region/city when present) comes from the Cloudflare CF-IPCountry header or a GeoLite-style database you provide. Rover does not bundle a geolocation database.", PeriodDays: days}
	cut := time.Now().UTC().AddDate(0, 0, -days+1).Format("2006-01-02")
	ents, err := os.ReadDir(analyticsDir(site))
	if err != nil {
		return out
	}
	seen := map[string]bool{}
	countries, regions, cities, devices, browsers, refs, pages, dayc := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	var engSum, engN int
	for _, e := range ents {
		day := strings.TrimSuffix(e.Name(), ".jsonl")
		if day < cut {
			continue
		}
		f, err := os.Open(filepath.Join(analyticsDir(site), e.Name()))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			var ev analyticsEvent
			if json.Unmarshal(sc.Bytes(), &ev) != nil {
				continue
			}
			if ev.Bot {
				out.Bots++
				if dropBots {
					continue
				}
			}
			out.Pageviews++
			out.Bandwidth += ev.Bytes
			if ev.Status >= 400 {
				out.Errors++
			}
			if ev.Unique != "" && !seen[ev.Unique] {
				seen[ev.Unique] = true
			}
			bump := func(m map[string]int, k string) {
				if k == "" {
					k = "Unknown"
				}
				m[k]++
			}
			bump(countries, ev.Country)
			if ev.Region != "" {
				bump(regions, ev.Region)
			}
			if ev.City != "" {
				bump(cities, ev.City)
			}
			bump(devices, ev.Device)
			bump(browsers, ev.Browser)
			if ev.Referrer != "" {
				bump(refs, ev.Referrer)
			}
			bump(pages, ev.Path)
			bump(dayc, ev.Day)
			if ev.EngageMs > 0 {
				engSum += ev.EngageMs
				engN++
			}
		}
		f.Close()
	}
	out.Uniques, out.Visitors = len(seen), len(seen)
	if engN > 0 {
		out.Engagement = engSum / engN
	}
	top := func(m map[string]int, n int) []analyticsCount {
		type kv struct {
			k string
			v int
		}
		var xs []kv
		for k, v := range m {
			xs = append(xs, kv{k, v})
		}
		for i := 0; i < len(xs); i++ {
			for j := i + 1; j < len(xs); j++ {
				if xs[j].v > xs[i].v {
					xs[i], xs[j] = xs[j], xs[i]
				}
			}
		}
		if len(xs) > n {
			xs = xs[:n]
		}
		var out []analyticsCount
		for _, x := range xs {
			out = append(out, analyticsCount{Name: x.k, Count: x.v})
		}
		return out
	}
	out.Countries, out.Regions, out.Cities = top(countries, 15), top(regions, 15), top(cities, 15)
	out.Devices, out.Browsers, out.Referrers, out.Pages, out.Days = top(devices, 8), top(browsers, 8), top(refs, 15), top(pages, 25), top(dayc, days)
	return out
}

func apiIngestLog(w http.ResponseWriter, r *http.Request) {
	// Accepts nginx combined / Rover log lines (one per line) for a site.
	// Used after an SSH pull. Does not require raw IPs to be stored: they
	// are hashed immediately.
	slug := r.PathValue("slug")
	c := snapshotConfig()
	if findSite(&c, slug) == nil {
		writeErr(w, fmt.Errorf("unknown project"))
		return
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeErr(w, err)
		return
	}
	n, err := ingestAccessLog(slug, string(b), c.Analytics == nil || c.Analytics.BotFilter)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ingested": n})
}

// roverLogRe matches: time path status bytes referrer ua country ip
// or a standard combined log line.
var combinedLogRe = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+)[^"]*" (\d{3}) (\d+|-) "([^"]*)" "([^"]*)"(?: "([^"]*)")?`)

func ingestAccessLog(site, text string, dropBots bool) (int, error) {
	n := 0
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := sc.Text()
		m := combinedLogRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ip, tstr, path, st, bytesS, ref, ua, country := m[1], m[2], m[4], m[5], m[6], m[7], m[8], ""
		if len(m) > 9 {
			country = m[9]
		}
		when, err := time.Parse("02/Jan/2006:15:04:05 -0700", tstr)
		if err != nil {
			when = time.Now().UTC()
		}
		day := when.UTC().Format("2006-01-02")
		status, _ := strconv.Atoi(st)
		var by int64
		if bytesS != "-" {
			by, _ = strconv.ParseInt(bytesS, 10, 64)
		}
		dev, br := classifyDevice(ua)
		ev := analyticsEvent{Day: day, Site: site, Path: path, Referrer: stripRef(ref), Country: strings.ToUpper(country), Device: dev, Browser: br, Unique: uniqueHash(day, ip, ua), Bytes: by, Status: status, Bot: isBot(ua)}
		if dropBots && ev.Bot {
			continue
		}
		if err := recordEvent(ev); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func apiAnalyticsGoogle(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	c := snapshotConfig()
	s := findSite(&c, slug)
	if s == nil {
		writeErr(w, fmt.Errorf("unknown project"))
		return
	}
	out := map[string]any{}
	kind := r.URL.Query().Get("source") // ga4 | search-console | adsense
	id := ""
	if s.Services != nil {
		id = s.Services["analytics"]
		if kind == "search-console" {
			id = firstNonEmpty(s.Services["seo"], id)
		}
		if kind == "adsense" {
			id = firstNonEmpty(s.Services["revenue"], id)
		}
	}
	if id == "" {
		writeJSON(w, 200, map[string]any{"unsupported": false, "connected": false, "error": "assign a Google Analytics, Search Console or AdSense account to this project in the Integration Centre"})
		return
	}
	a := findIntegration(&c, id)
	if a == nil {
		writeErr(w, fmt.Errorf("unknown account"))
		return
	}
	_ = kind
	_ = out
	writeJSON(w, 200, map[string]any{"connected": true, "provider": a.Provider, "who": a.Who, "note": "Reports are fetched with the official read-only APIs. Live numbers need a successful test of that account."})
}
