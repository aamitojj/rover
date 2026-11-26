//go:build shotseed

package main

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// TestShotSeed writes a demo config and deploy history for screenshots.
// Run: SEED_CFG=/tmp/dl06/cfg SEED_PROJ=/tmp/dl06/proj go test -tags shotseed -run ShotSeed .
func TestShotSeed(t *testing.T) {
	os.Setenv("ROVER_CONFIG_DIR", os.Getenv("SEED_CFG"))
	os.Setenv("ROVER_PROJECTS_DIR", os.Getenv("SEED_PROJ"))
	cfgMu.Lock()
	cfg = &Config{SSHPort: 22, SSHUser: "root"}
	cfgMu.Unlock()
	tok, _ := protectString("testtoken")
	now := time.Now()
	ts := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	on := true
	off := false
	_ = withConfig(func(c *Config) error {
		c.ConfigVersion = currentConfigVersion
		c.ServerIP, c.SSHPort, c.SSHUser = "203.0.113.10", 22, "root"
		c.CoolifyURL, c.CoolifyToken, c.ServerUUID = "http://127.0.0.1:18000", tok, "srv1"
		c.Servers = []*ServerProfile{
			{ID: "s1", Label: "Hetzner CPX31 (Nuremberg)", Provider: "hetzner", ServerIP: "203.0.113.10", SSHPort: 22, SSHUser: "root", CoolifyURL: "http://127.0.0.1:18000", CoolifyToken: tok, ServerUUID: "srv1", Added: ts(800 * time.Hour)},
			{ID: "s2", Label: "DigitalOcean 4 GB (Bangalore)", Provider: "digitalocean", ServerIP: "198.51.100.7", SSHPort: 22, SSHUser: "root", Added: ts(200 * time.Hour)},
		}
		c.Servers = append(c.Servers,
			&ServerProfile{ID: "home1", Label: "Raspberry Pi 4 (study)", Provider: "home", Kind: "home", Engine: "lite", Arch: "aarch64", RAMMB: 3796, Reach: "tunnel", TunnelID: "7d1c2b9e-home", ServerIP: "192.168.1.50", SSHPort: 22, SSHUser: "pi", Added: ts(50 * time.Hour)},
			&ServerProfile{ID: thisPCID, Label: "This PC", Provider: "thispc", Kind: "thispc", ServerIP: "127.0.0.1", Added: ts(20 * time.Hour)})
		c.CloudflareToken = tok
		c.ActiveServer = "s1"
		c.SetupHidden = true
		c.RecentFolders = []string{`C:\Users\Amit\Sites\bakery\dist`, `C:\Users\Amit\Sites\portfolio\build`, `C:\Users\Amit\Downloads\docs-site.zip`}
		c.Sites = []*Site{
			{Slug: "bakery", Name: "Bakery", ProjectUUID: "p1", AppUUID: "a1", Domain: "sweetcrumbs.in", IncludeWWW: true, DomainLive: true, HTTPSActive: true, InstantURL: "https://bakery.203.0.113.10.sslip.io", CreatedAt: ts(600 * time.Hour), Server: "s1", Pinned: true, Tags: []string{"client"}, ConfMount: true, LastSource: `C:\Users\Amit\Sites\bakery\dist`, Primary: "www", AlertDown: &on,
				Servers: []string{"s2", "home1"}, HAMode: "backup", DNSMode: "cloudflare", CFZoneID: "zone1", Failover: &FailoverSettings{Auto: true, SwitchBack: true, Active: "s1"}},
			{Slug: "portfolio", Name: "Portfolio", ProjectUUID: "p2", AppUUID: "a2", InstantURL: "https://portfolio.203.0.113.10.sslip.io", CreatedAt: ts(30 * time.Hour), Server: "s1", ConfMount: true},
			{Slug: "docs-site", Name: "Docs site", ProjectUUID: "p3", AppUUID: "a3", Domain: "docs.acme-tools.com", DomainLive: true, HTTPSActive: true, InstantURL: "https://docs-site.203.0.113.10.sslip.io", CreatedAt: ts(300 * time.Hour), Server: "s1", Locked: true, BuildCmd: "npm run build", PublishDir: "dist", ConfMount: true, AlertFail: &off},
			{Slug: "clinic", Name: "City Clinic", ProjectUUID: "p4", AppUUID: "a4", InstantURL: "https://clinic.198.51.100.7.sslip.io", CreatedAt: ts(100 * time.Hour), Server: "s2", ConfMount: true},
		}
		return nil
	})
	page := func(title, body string) []srcEntry {
		files := map[string]string{
			"index.html":  "<!doctype html><html><head><title>" + title + "</title><link rel=stylesheet href=style.css></head><body><h1>" + title + "</h1><p>" + body + "</p><img src=logo.png alt=logo></body></html>",
			"style.css":   "body{font-family:sans-serif;margin:40px}h1{color:#b5651d}",
			"about.html":  "<!doctype html><html><head><title>About</title></head><body>About us</body></html>",
			"logo.png":    strings.Repeat("P", 2048),
			"_redirects":  "/menu  /index.html#menu  301\n",
			"js/app.js":   "console.log('" + title + "')",
			"404.html":    "<!doctype html><title>Not found</title>",
			"robots.txt":  "User-agent: *\n",
			"sitemap.xml": "<urlset></urlset>",
			"favicon.ico": "ico",
		}
		var e []srcEntry
		for p, b := range files {
			b := b
			e = append(e, srcEntry{Path: p, Size: int64(len(b)), Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(b)), nil }})
		}
		return e
	}
	deploy := func(slug, title, body string, draft bool) {
		ws, err := newWorkspace(page(title, body), "drop")
		if err != nil {
			t.Fatal(err)
		}
		defer ws.Close()
		ws.draft = draft
		rec := archiveDeploy(&Job{}, slug, ws)
		if rec == nil {
			t.Fatal("no record")
		}
		if !draft {
			rel := strings.SplitN(rec.ID, "_", 2)[0]
			_ = withConfig(func(c *Config) error {
				s := findSite(c, slug)
				s.Release, s.LastDeploy = rel, time.Now().Format(time.RFC3339)
				return nil
			})
			finishDeploy(slug, rec.ID, nil)
		}
		time.Sleep(1100 * time.Millisecond)
	}
	deploy("bakery", "Sweet Crumbs", "Fresh bread daily.", false)
	deploy("bakery", "Sweet Crumbs", "Fresh bread and cakes daily.", false)
	deploy("bakery", "Sweet Crumbs", "Now open Sundays!", false)
	deploy("bakery", "Sweet Crumbs", "Christmas menu (draft)", true)
	deploy("docs-site", "Acme Docs", "v2 docs", false)
	deploy("docs-site", "Acme Docs", "v3 docs from Git", true)
	deploy("clinic", "City Clinic", "Book an appointment", false)
}
