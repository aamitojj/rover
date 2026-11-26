package main

// Rover 0.7: automatic failover ("Primary + backup" mode).
//
// Rover checks every server of the project each minute. When the server
// that visitors are sent to fails 3 checks in a row and another server
// passed its checks, Rover points the domain at that server through the
// Cloudflare API (proxied record, so the change is live in seconds) and
// tells the user. When the main server passes 3 checks in a row again it
// switches back (if "Switch back automatically" is on).
//
// This needs Rover running somewhere (it is the one doing the checks). For
// switching that works while this PC is off, use "Together" mode, where
// Cloudflare itself routes around a server that is down.

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type FailoverSettings struct {
	Auto       bool            `json:"auto"`        // switch without asking
	SwitchBack bool            `json:"switch_back"` // return to the main server when it recovers
	Active     string          `json:"active,omitempty"`
	LastSwitch string          `json:"last_switch,omitempty"`
	Events     []FailoverEvent `json:"events,omitempty"`
}

type FailoverEvent struct {
	Time string `json:"time"`
	Text string `json:"text"`
}

const (
	foDownAfter = 3
	foUpAfter   = 3
)

// healthState is the in-memory check history of one server for one project.
type healthState struct {
	Known   bool      `json:"known"`
	Up      bool      `json:"up"`
	Fails   int       `json:"fails"`
	OKs     int       `json:"oks"`
	LastErr string    `json:"last_err,omitempty"`
	Checked time.Time `json:"checked"`
}

func (h *healthState) record(err error, now time.Time) {
	h.Checked = now
	if err == nil {
		h.OKs++
		h.Fails = 0
		h.LastErr = ""
		if !h.Known || h.OKs >= foUpAfter {
			h.Up = true
		}
		h.Known = true
		return
	}
	h.Fails++
	h.OKs = 0
	h.LastErr = shortErr(err)
	if h.Fails >= foDownAfter {
		h.Up, h.Known = false, true
	}
}

var srvHealthMu = struct {
	sync.Mutex
	m       map[string]map[string]*healthState // slug -> server -> state
	alerted map[string]string                  // slug -> last alert key (no repeats)
}{m: map[string]map[string]*healthState{}, alerted: map[string]string{}}

func srvHealth(slug, id string) *healthState {
	srvHealthMu.Lock()
	defer srvHealthMu.Unlock()
	if srvHealthMu.m[slug] == nil {
		srvHealthMu.m[slug] = map[string]*healthState{}
	}
	h := srvHealthMu.m[slug][id]
	if h == nil {
		h = &healthState{}
		srvHealthMu.m[slug][id] = h
	}
	return h
}

func healthOf(slug, id string) string {
	h := srvHealth(slug, id)
	srvHealthMu.Lock()
	defer srvHealthMu.Unlock()
	switch {
	case !h.Known:
		return "unknown"
	case h.Up:
		return "up"
	}
	return "down"
}

func failoverActive(s *Site) string {
	if s.Failover != nil && s.Failover.Active != "" {
		for _, id := range siteServerIDs(s) {
			if id == s.Failover.Active {
				return id
			}
		}
	}
	return s.Server
}

// foDecision is what one round of checks asks for.
type foDecision struct {
	SwitchTo string // "" = stay
	Reason   string
	AllDown  bool
	Ask      bool // a switch is needed but automatic switching is off
}

// foStep decides from the health of every server (order = main first).
func foStep(cfg FailoverSettings, order []string, h map[string]*healthState) foDecision {
	if len(order) == 0 {
		return foDecision{}
	}
	active := cfg.Active
	if active == "" {
		active = order[0]
	}
	upKnown := func(id string) bool { x := h[id]; return x != nil && x.Known && x.Up }
	down := func(id string) bool { x := h[id]; return x != nil && x.Known && !x.Up }
	if down(active) {
		for _, id := range order {
			if id != active && upKnown(id) {
				return foDecision{SwitchTo: id, Reason: "down", Ask: !cfg.Auto}
			}
		}
		all := true
		for _, id := range order {
			if !down(id) {
				all = false
			}
		}
		return foDecision{AllDown: all}
	}
	if cfg.SwitchBack && active != order[0] && upKnown(order[0]) {
		x := h[order[0]]
		if x.OKs >= foUpAfter {
			return foDecision{SwitchTo: order[0], Reason: "back", Ask: !cfg.Auto}
		}
	}
	return foDecision{}
}

// ---------- probes ----------

// probeServer checks that one server serves the project (tests replace it).
var probeServer = func(c *Config, s *Site, p *ServerProfile) error {
	host := s.Slug + ".localhost"
	if hs := siteHosts(s); s.Domain != "" && len(hs) > 0 {
		host = hs[0]
	}
	if p.Kind == "thispc" {
		return httpProbe(fmt.Sprintf("http://127.0.0.1:%d/", thisPCPort(c)), host)
	}
	if p.ServerIP == "" {
		return errors.New("no address")
	}
	if p.Engine != "lite" && s.Domain == "" {
		if r := replicaOf(s, p.ID); r != nil && r.InstantURL != "" {
			host = strings.TrimPrefix(strings.TrimPrefix(r.InstantURL, "https://"), "http://")
		} else if p.ID == s.Server && s.InstantURL != "" {
			host = strings.TrimPrefix(strings.TrimPrefix(s.InstantURL, "https://"), "http://")
		}
	}
	err := httpProbe("http://"+net.JoinHostPort(p.ServerIP, "80")+"/", host)
	if err == nil {
		return nil
	}
	var ne net.Error
	if !errors.As(err, &ne) && !strings.Contains(err.Error(), "connect") {
		return err // the server answered, but with an error
	}
	// No direct way in (home server behind a tunnel, firewall): ask the
	// machine itself over SSH.
	r, rerr := runnerFor(c, p)
	if rerr != nil {
		return err
	}
	out, rerr := runOut(r, fmt.Sprintf(`curl -sk -o /dev/null -w '%%{http_code}' -H %s https://127.0.0.1/ || true`, shellQuote("Host: "+host)))
	code := strings.TrimSpace(out)
	if rerr == nil && (strings.HasPrefix(code, "2") || strings.HasPrefix(code, "3") || code == "401") {
		return nil
	}
	return fmt.Errorf("not answering (HTTP %s)", firstNonEmpty(code, "none"))
}

func httpProbe(url, host string) error {
	cl := &http.Client{
		Timeout:       8 * time.Second,
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Host = host
	req.Header.Set("User-Agent", "Rover-health/1")
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode < 400 || resp.StatusCode == 401 {
		return nil
	}
	return fmt.Errorf("HTTP %d", resp.StatusCode)
}

// pcOnline: never fail over because this PC lost its own internet.
var pcOnline = func() bool {
	d := net.Dialer{Timeout: 4 * time.Second}
	for _, a := range []string{"1.1.1.1:443", "8.8.8.8:443", "api.cloudflare.com:443"} {
		if c, err := d.Dial("tcp", a); err == nil {
			c.Close()
			return true
		}
	}
	return false
}

// ---------- switching ----------

// switchReady says whether Rover can switch the project's DNS itself.
func switchReady(c *Config, s *Site) error {
	if s.Domain == "" {
		return errors.New("connect a domain to the project first")
	}
	if c.CloudflareToken == "" || s.CFZoneID == "" {
		return errors.New("automatic switching needs the domain on Cloudflare (free plan) with Rover's Cloudflare connection. Until then, change the DNS record at your registrar (Rover shows the steps)")
	}
	return nil
}

// failoverSwitch points the domain at another server of the project.
var failoverSwitch = func(c *Config, s *Site, to string) error {
	if err := switchReady(c, s); err != nil {
		return err
	}
	p := findServer(c, to)
	if p == nil {
		return errors.New("unknown server")
	}
	typ, content := "A", p.ServerIP
	switch {
	case p.TunnelID != "":
		typ, content = "CNAME", tunnelTarget(p.TunnelID)
	case p.Kind == "thispc":
		return errors.New("This PC is not connected to Cloudflare yet: open This PC and click Connect to the internet")
	case isPrivateHost(p.ServerIP):
		return fmt.Errorf("%s has only a home-network address; connect it to Cloudflare (tunnel) first", p.Label)
	}
	cf := newCF(secret(c.CloudflareToken))
	var ids []string
	for _, h := range siteHosts(s) {
		id, err := cfPoint(cf, s.CFZoneID, h, typ, content)
		if err != nil {
			return fmt.Errorf("Cloudflare: %w", err)
		}
		ids = append(ids, id)
	}
	if p.TunnelID != "" {
		_ = syncServerTunnelConfig(c, to)
	}
	return withConfig(func(c *Config) error {
		x := findSite(c, s.Slug)
		if x == nil {
			return nil
		}
		if x.Failover == nil {
			x.Failover = &FailoverSettings{Auto: true, SwitchBack: true}
		}
		x.Failover.Active, x.Failover.LastSwitch = to, time.Now().Format(time.RFC3339)
		x.CFRecordIDs, x.CFProxy = ids, true
		return nil
	})
}

func addFailoverEvent(slug, text string) {
	_ = withConfig(func(c *Config) error {
		s := findSite(c, slug)
		if s == nil {
			return nil
		}
		if s.Failover == nil {
			s.Failover = &FailoverSettings{Auto: true, SwitchBack: true}
		}
		s.Failover.Events = append([]FailoverEvent{{Time: time.Now().Format(time.RFC3339), Text: text}}, s.Failover.Events...)
		if len(s.Failover.Events) > 20 {
			s.Failover.Events = s.Failover.Events[:20]
		}
		return nil
	})
}

// ---------- the background round ----------

var foLast time.Time

func failoverTick(now time.Time) {
	if now.Sub(foLast) < 55*time.Second {
		return
	}
	foLast = now
	c := snapshotConfig()
	var sites []*Site
	for _, s := range c.Sites {
		if multiServer(s) && s.Release != "" {
			sites = append(sites, s)
		}
	}
	if len(sites) == 0 || !pcOnline() {
		return
	}
	for _, s := range sites {
		checkProject(&c, s, now)
	}
}

// checkProject probes every server of one project and acts on the result.
func checkProject(c *Config, s *Site, now time.Time) {
	ids := siteServerIDs(s)
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		p := findServer(c, id)
		if p == nil {
			continue
		}
		wg.Add(1)
		go func(i int, p *ServerProfile) {
			defer wg.Done()
			errs[i] = probeServer(c, s, p)
		}(i, p)
	}
	wg.Wait()
	states := map[string]*healthState{}
	for i, id := range ids {
		h := srvHealth(s.Slug, id)
		srvHealthMu.Lock()
		wasUp, wasKnown := h.Up, h.Known
		h.record(errs[i], now)
		cp := *h
		srvHealthMu.Unlock()
		states[id] = &cp
		if wasKnown && wasUp && cp.Known && !cp.Up {
			notify("warn", s.Slug, fmt.Sprintf("%s: %s stopped answering (%s).", s.Name, serverLabel(c, id), cp.LastErr))
		} else if wasKnown && !wasUp && cp.Up {
			notify("info", s.Slug, fmt.Sprintf("%s: %s is answering again.", s.Name, serverLabel(c, id)))
		}
	}
	if s.HAMode != "backup" {
		return // Together mode: Cloudflare routes around a server that is down
	}
	cfg := FailoverSettings{Auto: true, SwitchBack: true}
	if s.Failover != nil {
		cfg = *s.Failover
	}
	cfg.Active = failoverActive(s)
	d := foStep(cfg, ids, states)
	alertKey := ""
	switch {
	case d.AllDown:
		alertKey = "alldown"
		if alertOnce(s.Slug, alertKey) {
			notify("error", s.Slug, fmt.Sprintf("%s: every server is down. Rover keeps checking.", s.Name))
		}
	case d.SwitchTo != "" && (d.Ask || switchReady(c, s) != nil):
		alertKey = "ask-" + d.SwitchTo
		if alertOnce(s.Slug, alertKey) {
			why := "is down"
			if d.Reason == "back" {
				why = "is back up"
			}
			notify("warn", s.Slug, fmt.Sprintf("%s: %s %s. Open the project's Hosting tab and click Switch now to send visitors to %s.", s.Name, serverLabel(c, map[string]string{"down": cfg.Active, "back": ids[0]}[d.Reason]), why, serverLabel(c, d.SwitchTo)))
		}
	case d.SwitchTo != "":
		from := cfg.Active
		if err := failoverSwitch(c, s, d.SwitchTo); err != nil {
			alertKey = "fail-" + d.SwitchTo
			if alertOnce(s.Slug, alertKey) {
				notify("error", s.Slug, fmt.Sprintf("%s: could not switch to %s: %v", s.Name, serverLabel(c, d.SwitchTo), err))
			}
			return
		}
		msg := fmt.Sprintf("Switched visitors from %s to %s (%s).", serverLabel(c, from), serverLabel(c, d.SwitchTo), map[string]string{"down": "the server stopped answering", "back": "the main server is back"}[d.Reason])
		addFailoverEvent(s.Slug, msg)
		notify("warn", s.Slug, s.Name+": "+msg)
	}
	if alertKey == "" {
		alertOnce(s.Slug, "")
	}
}

// alertOnce returns true the first time key is seen in a row.
func alertOnce(slug, key string) bool {
	srvHealthMu.Lock()
	defer srvHealthMu.Unlock()
	if srvHealthMu.alerted[slug] == key {
		return false
	}
	srvHealthMu.alerted[slug] = key
	return key != ""
}

func srvHealthSnapshot(slug string) map[string]healthState {
	srvHealthMu.Lock()
	defer srvHealthMu.Unlock()
	out := map[string]healthState{}
	for k, v := range srvHealthMu.m[slug] {
		out[k] = *v
	}
	return out
}

// checkNow runs one round for a project immediately (Check now button).
func checkNow(slug string) error {
	c := snapshotConfig()
	s := findSite(&c, slug)
	if s == nil {
		return errors.New("unknown project")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { checkProject(&c, s, time.Now()); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return errors.New("checks are taking long; results will appear shortly")
	}
}
