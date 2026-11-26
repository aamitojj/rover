package main

// Rover 0.5: Jiv's offline knowledge base must answer real questions
// (English, Hinglish, Hindi, typos) with the right article first.

import (
	"strings"
	"testing"
)

var kbCases = []struct{ q, screen, want string }{
	{"how do i deploy a new build", "", "drop-deploy"},
	{"password protect my site", "", "password-protect"},
	{"turn on automatic updates", "", "updates"},
	{"lock publishing", "", "drafts-lock"},
	{"redirect www to non www", "", "primary-domain"},
	{"what is on the project page", "", "project-hub"},
	{"how do i add my domain", "", "add-domain"},
	{"connect godaddy domain", "", "godaddy"},
	{"my godadyy domain is not working", "", "godaddy"},
	{"site shows old version after deploy", "", "old-content"},
	{"why is my website down", "", "site-down"},
	{"https padlock not showing", "", "https-ssl"},
	{"ssl certificate error", "", "https-ssl"},
	{"how to undo a deploy", "", "undo|rollback"},
	{"roll back to the last version", "", "rollback"},
	{"go back to previous version", "", "rollback"},
	{"ssh connection failed", "", "ssh-failed"},
	{"host key changed warning", "", "host-key"},
	{"server disk is full", "", "server-full"},
	{"how do I connect my hetzner account", "", "hosting-accounts"},
	{"which providers can create servers", "", "hosting-providers-support"},
	{"create a new vps", "", "create-server"},
	{"use another server for one project", "", "multiple-servers"},
	{"firewall ports blocked", "", "firewall-ports"},
	{"coolify token", "", "coolify-token"},
	{"keyboard shortcuts", "", "shortcuts"},
	{"import from netlify", "", "netlify-migrate"},
	{"deploy from github repo", "", "git-import"},
	{"private repository error", "", "error-git-private"},
	{"no index.html found", "", "error-index-missing"},
	{"upload too large", "", "error-too-large"},
	{"too many requests 429", "", "error-rate-limit"},
	{"npm not found build", "", "error-node-missing"},
	{"offline brain download", "", "offline-brain"},
	{"does jiv send my data to the cloud", "", "jiv-privacy"},
	{"dns not updated yet", "", "dns-waiting"},
	{"cloudflare nameservers", "", "cloudflare"},
	{"email mx records", "", "email-dns"},
	{"delete a project", "", "delete-project"},
	{"backup my sites", "", "backups-alerts"},
	{"redirects and headers", "", "redirects-headers"},
	{"test my site before going live", "", "app-testing"},
	{"update rover to new version", "", "updates"},
	{"domain kaise jode", "", "add-domain"},
	{"site band hai kya karu", "", "site-down"},
	{"डोमेन कैसे जोड़ें", "", "add-domain"},
	{"what can I do here", "logs", "logs-tab"},
	{"keep my site online if the server goes down", "", "failover-backup"},
	{"add a backup server", "", "failover-backup"},
	{"run one project on two servers at the same time", "", "failover-backup"},
	{"deploy failed on one server", "", "partial-deploy"},
	{"servers out of sync", "", "partial-deploy"},
	{"use my own server at home", "", "home-server"},
	{"raspberry pi hosting", "", "home-server"},
	{"no public ip cgnat", "", "cloudflare-tunnel"},
	{"port forwarding not working", "", "cloudflare-tunnel"},
	{"host website on this pc", "", "this-pc"},
	{"wifi down use mobile data backup", "", "connection-backup"},
	{"together mode", "", "together-mode"},
	{"ghar ka server use karna", "", "home-server"},
}

func TestKBSearchTop1(t *testing.T) {
	fails := 0
	for _, c := range kbCases {
		h := jivKB().Search(c.q, c.screen, 3)
		got := ""
		if len(h) > 0 {
			got = h[0].ID
		}
		if !strings.Contains("|"+c.want+"|", "|"+got+"|") {
			fails++
			alt := []string{}
			for _, x := range h {
				alt = append(alt, x.ID)
			}
			t.Errorf("%q: got %v, want %s", c.q, alt, c.want)
		}
	}
	if len(kbCases) < 30 {
		t.Fatal("need 30+ cases")
	}
	t.Logf("%d/%d top-1", len(kbCases)-fails, len(kbCases))
}

func TestKBIntegrity(t *testing.T) {
	ix := jivKB()
	if len(ix.docs) < 55 {
		t.Fatalf("only %d articles", len(ix.docs))
	}
	seen := map[string]bool{}
	for _, a := range ix.docs {
		if a.ID == "" || a.Title == "" || len(a.Body) < 80 {
			t.Errorf("article %q too thin", a.ID)
		}
		if seen[a.ID] {
			t.Errorf("duplicate id %s", a.ID)
		}
		seen[a.ID] = true
		for _, g := range a.Go {
			if len(g.Href) < 2 || g.Href[0] != '#' {
				t.Errorf("%s: bad link %q", a.ID, g.Href)
			}
		}
	}
	if !kbConfident(ix.Search("add domain", "", 3)) {
		t.Error("a clear question should be confident")
	}
	if kbConfident(ix.Search("banana bread recipe", "", 3)) {
		t.Error("off-topic must not be confident")
	}
}
