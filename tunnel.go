package main

// Rover 0.7: Cloudflare Tunnel.
//
// A tunnel is an outgoing connection from a server to Cloudflare, so a site
// works without a public IP or open ports (home internet, CGNAT, mobile data).
// Rover uses remotely-managed tunnels (the ingress rules live at
// Cloudflare) and runs cloudflared with the tunnel token.
//
// API (developers.cloudflare.com/api/resources/zero_trust/subresources/tunnels):
//   GET  /accounts                                         (find the account)
//   POST /accounts/{acc}/cfd_tunnel {"name","config_src":"cloudflare"}
//   GET  /accounts/{acc}/cfd_tunnel/{id}                   (status, connections)
//   GET  /accounts/{acc}/cfd_tunnel/{id}/token
//   PUT  /accounts/{acc}/cfd_tunnel/{id}/configurations    {"config":{"ingress":[...]}}
//   DELETE /accounts/{acc}/cfd_tunnel/{id}
// DNS: a proxied CNAME <host> -> <id>.cfargotunnel.com.
// Token permissions: Account > Cloudflare Tunnel > Edit, Zone > DNS > Edit.
//
// Two kinds of tunnels:
//   - a server tunnel ("dilreet-srv-<label>") gives one home server a way in
//     for every project on it (one connector per machine);
//   - a project tunnel ("dilreet-<slug>") with connectors on every server of a
//     project: "Together" mode. Cloudflare sends visitors to a connector that
//     is up, so the site keeps working when a server is off, even while this
//     PC is off.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"
)

// pinned cloudflared release (github.com/cloudflare/cloudflared, Apache-2.0).
const cloudflaredVersion = "2026.11.2"

var cloudflaredSHA256 = map[string]string{
	"windows-amd64.exe": "555b46c85582461d189cac30d14ad05480d612613801be9f110b8ce08c5b2113",
	"linux-amd64":       "56f13335143ec0e22772963290683a84a04114a35ec64cbc794a4d39ff9bb8cf",
	"linux-arm64":       "04a7b7f08bce85d56b26ffe1fdbfbb21a074495a363d16f0783c40e25b5c5281",
	"linux-arm":         "d219494581180764c6d2454409691bb06879d01b43cdeae12476070a7e4e4762",
	"linux-armhf":       "ab8dda127892266ef25d9787ea43be2314832204e2e32272901ae36010454684",
}

func cloudflaredURL(asset string) string {
	return "https://github.com/cloudflare/cloudflared/releases/download/" + cloudflaredVersion + "/cloudflared-" + asset
}

// ---------- API ----------

type CFTunnel struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Status      string         `json:"status"` // inactive | degraded | healthy | down
	Token       string         `json:"token,omitempty"`
	Connections []CFTunnelConn `json:"connections,omitempty"`
}

type CFTunnelConn struct {
	ColoName string `json:"colo_name"`
	OriginIP string `json:"origin_ip"`
	ClientID string `json:"client_id"`
	Opened   string `json:"opened_at"`
}

func (c *cfClient) CreateTunnel(acc, name string) (CFTunnel, error) {
	var t CFTunnel
	return t, c.do("POST", "/accounts/"+acc+"/cfd_tunnel", map[string]any{"name": name, "config_src": "cloudflare"}, &t)
}

func (c *cfClient) Tunnel(acc, id string) (CFTunnel, error) {
	var t CFTunnel
	return t, c.do("GET", "/accounts/"+acc+"/cfd_tunnel/"+id, nil, &t)
}

func (c *cfClient) TunnelToken(acc, id string) (string, error) {
	var s string
	return s, c.do("GET", "/accounts/"+acc+"/cfd_tunnel/"+id+"/token", nil, &s)
}

func (c *cfClient) PutTunnelConfig(acc, id string, ingress []IngressRule) error {
	return c.do("PUT", "/accounts/"+acc+"/cfd_tunnel/"+id+"/configurations", map[string]any{"config": map[string]any{"ingress": ingress}}, nil)
}

func (c *cfClient) DeleteTunnel(acc, id string) error {
	return c.do("DELETE", "/accounts/"+acc+"/cfd_tunnel/"+id, nil, nil)
}

// ---------- ingress ----------

type IngressRule struct {
	Hostname      string         `json:"hostname,omitempty"`
	Service       string         `json:"service"`
	OriginRequest *OriginRequest `json:"originRequest,omitempty"`
}

type OriginRequest struct {
	NoTLSVerify      bool   `json:"noTLSVerify,omitempty"`
	OriginServerName string `json:"originServerName,omitempty"`
	HTTPHostHeader   string `json:"httpHostHeader,omitempty"`
}

// tunnelIngress builds the ingress rules for a set of hostnames. Coolify
// (Traefik) and lite nginx both answer HTTPS on 443 with the right host
// name; the certificate on the server is not checked (Cloudflare holds the
// real certificate). The last rule must be the catch-all.
func tunnelIngress(service string, hosts []string) []IngressRule {
	seen := map[string]bool{}
	var hs []string
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" && !seen[h] {
			seen[h] = true
			hs = append(hs, h)
		}
	}
	sort.Strings(hs)
	var out []IngressRule
	for _, h := range hs {
		r := IngressRule{Hostname: h, Service: service}
		if strings.HasPrefix(service, "https://") {
			r.OriginRequest = &OriginRequest{NoTLSVerify: true, OriginServerName: h, HTTPHostHeader: h}
		} else {
			r.OriginRequest = &OriginRequest{HTTPHostHeader: h}
		}
		out = append(out, r)
	}
	return append(out, IngressRule{Service: "http_status:404"})
}

// serviceFor is where cloudflared on a server sends visitors.
func serviceFor(p *ServerProfile, c *Config) string {
	if p != nil && p.Kind == "thispc" {
		return "http://localhost:" + fmt.Sprint(thisPCPort(c))
	}
	return "https://localhost:443"
}

func tunnelTarget(id string) string { return id + ".cfargotunnel.com" }

// ---------- account / client helpers ----------

func cfFromConfig(c *Config) (*cfClient, error) {
	if c.CloudflareToken == "" {
		return nil, errors.New("connect Cloudflare first (Settings > Cloudflare). The free plan is enough")
	}
	return newCF(secret(c.CloudflareToken)), nil
}

// cfAccount finds (and remembers) the Cloudflare account id.
func cfAccount(c *Config, cf *cfClient) (string, error) {
	if c.CFAccountID != "" {
		return c.CFAccountID, nil
	}
	acc, err := cf.Accounts()
	if err != nil {
		return "", fmt.Errorf("%v. The Cloudflare token needs the permission Account > Cloudflare Tunnel > Edit", err)
	}
	if len(acc) == 0 {
		return "", errors.New("the Cloudflare token cannot see any account: add the permission Account > Cloudflare Tunnel > Edit")
	}
	id := acc[0].ID
	_ = withConfig(func(c *Config) error { c.CFAccountID = id; return nil })
	return id, nil
}

// cfPoint makes fqdn point to content with exactly one A/AAAA/CNAME record
// (proxied). Used by tunnels and by automatic failover.
func cfPoint(cf *cfClient, zone, fqdn, typ, content string) (string, error) {
	recs, err := cf.Records(zone, fqdn)
	if err != nil {
		return "", err
	}
	var keep *CFRecord
	for i := range recs {
		r := recs[i]
		if r.Type != "A" && r.Type != "AAAA" && r.Type != "CNAME" {
			continue
		}
		if keep == nil && r.Type == typ {
			keep = &recs[i]
			continue
		}
		if err := cf.Delete(zone, r.ID); err != nil {
			return "", err
		}
	}
	if keep != nil {
		if keep.Content == content && keep.Proxied {
			return keep.ID, nil
		}
		out, err := cf.Patch(zone, keep.ID, map[string]any{"content": content, "proxied": true, "ttl": 1})
		return out.ID, err
	}
	out, err := cf.Create(zone, CFRecord{Type: typ, Name: fqdn, Content: content, TTL: 1, Proxied: true, Comment: "Managed by Rover"})
	return out.ID, err
}

// ---------- cloudflared on Linux servers ----------

// cloudflaredAsset maps uname -m to the release asset.
func cloudflaredAsset(arch string) string {
	switch strings.ToLower(strings.TrimSpace(arch)) {
	case "x86_64", "amd64":
		return "linux-amd64"
	case "aarch64", "arm64":
		return "linux-arm64"
	case "armv7l", "armhf":
		return "linux-armhf"
	case "armv6l", "arm":
		return "linux-arm"
	}
	return ""
}

// cloudflaredInstallScript installs the pinned cloudflared (checksum
// checked) and a systemd service that runs the tunnel. The token is read
// from a root-only file (never on a command line). HP_DRY=1 prints the
// commands instead (tests).
func cloudflaredInstallScript(unit, tokenFile string) string {
	var sums strings.Builder
	for _, k := range []string{"linux-amd64", "linux-arm64", "linux-armhf", "linux-arm"} {
		fmt.Fprintf(&sums, "  %s) SUM=%s ;;\n", k, cloudflaredSHA256[k])
	}
	return fmt.Sprintf(`set -eu
run() { if [ -n "${HP_DRY:-}" ]; then echo "+ $*"; else "$@"; fi; }
UNIT=%[1]s
TOKFILE=%[2]s
VER=%[3]s
case "$(uname -m)" in
  x86_64|amd64) ASSET=linux-amd64 ;;
  aarch64|arm64) ASSET=linux-arm64 ;;
  armv7l|armhf) ASSET=linux-armhf ;;
  armv6l) ASSET=linux-arm ;;
  *) echo "unsupported processor: $(uname -m)" >&2; exit 1 ;;
esac
case "$ASSET" in
%[4]s esac
BIN=/usr/local/bin/cloudflared
if [ -x "$BIN" ] && "$BIN" --version 2>/dev/null | grep -q "$VER"; then
  echo "cloudflared $VER already installed"
else
  echo "Downloading cloudflared $VER ($ASSET)..."
  run curl -fsSL -o /tmp/cloudflared.dl "https://github.com/cloudflare/cloudflared/releases/download/$VER/cloudflared-$ASSET"
  if [ -z "${HP_DRY:-}" ]; then
    echo "$SUM  /tmp/cloudflared.dl" | sha256sum -c - >/dev/null || { rm -f /tmp/cloudflared.dl; echo "checksum mismatch: download rejected" >&2; exit 1; }
  else
    echo "+ sha256sum -c ($SUM)"
  fi
  run install -m 0755 /tmp/cloudflared.dl "$BIN"
  run rm -f /tmp/cloudflared.dl
fi
run mkdir -p /etc/dilreet
if [ -z "${HP_DRY:-}" ]; then
  umask 077
  printf 'TUNNEL_TOKEN=%%s\n' "$(cat "$TOKFILE")" > /etc/dilreet/$UNIT.env
  rm -f "$TOKFILE"
  cat > /etc/systemd/system/$UNIT.service <<HPUNIT
[Unit]
Description=Rover Cloudflare tunnel ($UNIT)
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/etc/dilreet/$UNIT.env
ExecStart=$BIN tunnel --no-autoupdate run
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
HPUNIT
else
  echo "+ write /etc/dilreet/$UNIT.env (token, mode 600)"
  echo "+ write /etc/systemd/system/$UNIT.service"
fi
run systemctl daemon-reload
run systemctl enable --now $UNIT
run systemctl restart $UNIT
echo "tunnel service $UNIT running"
`, shellQuote(unit), shellQuote(tokenFile), cloudflaredVersion, sums.String())
}

func tunnelUnit(id string) string {
	if len(id) > 8 {
		id = id[:8]
	}
	return "dilreet-tunnel-" + id
}

// installConnector starts cloudflared for a tunnel on one server.
var installConnector = func(j *Job, c *Config, p *ServerProfile, tunnelID, token string) error {
	if p.Kind == "thispc" {
		return localTunnels.start(tunnelID, token)
	}
	r, err := runnerFor(c, p)
	if err != nil {
		return err
	}
	sr, ok := r.(sshRunner)
	if !ok {
		return errors.New("internal: no SSH session")
	}
	tok, err := sr.st.uploadSecret([]byte(token))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	w := &prefixWriter{j: j, prefix: "[" + p.Label + "] "}
	err = r.Run(ctx, cloudflaredInstallScript(tunnelUnit(tunnelID), tok), w)
	w.flush()
	return err
}

// removeConnector stops a tunnel service on a server.
var removeConnector = func(c *Config, p *ServerProfile, tunnelID string) error {
	if p.Kind == "thispc" {
		localTunnels.stop(tunnelID)
		return nil
	}
	r, err := runnerFor(c, p)
	if err != nil {
		return err
	}
	u := tunnelUnit(tunnelID)
	_, err = runOut(r, fmt.Sprintf("systemctl disable --now %[1]s 2>/dev/null || true; rm -f /etc/systemd/system/%[1]s.service /etc/dilreet/%[1]s.env; systemctl daemon-reload", u))
	return err
}

// ---------- server tunnels (home servers) ----------

// ensureServerTunnel gives a home server its own tunnel and connector.
func ensureServerTunnel(j *Job, serverID string) (string, error) {
	c := snapshotConfig()
	p := findServer(&c, serverID)
	if p == nil {
		return "", errors.New("unknown server")
	}
	cf, err := cfFromConfig(&c)
	if err != nil {
		return "", err
	}
	acc, err := cfAccount(&c, cf)
	if err != nil {
		return "", err
	}
	id, token := p.TunnelID, secret(p.TunnelToken)
	if id == "" {
		j.Logf("Creating a Cloudflare tunnel for %s...", p.Label)
		t, err := cf.CreateTunnel(acc, "dilreet-srv-"+liteKey(strings.ToLower(p.Label))+"-"+randHex(2))
		if err != nil {
			return "", err
		}
		id, token = t.ID, t.Token
		if token == "" {
			if token, err = cf.TunnelToken(acc, id); err != nil {
				return "", err
			}
		}
		enc, err := protectString(token)
		if err != nil {
			return "", err
		}
		_ = withConfig(func(c *Config) error {
			if x := findServer(c, serverID); x != nil {
				x.TunnelID, x.TunnelToken, x.Reach = id, enc, "tunnel"
			}
			return nil
		})
	}
	if err := syncServerTunnelConfig(&c, serverID); err != nil {
		j.Logf("Note: %v", err)
	}
	j.Logf("Starting the tunnel on %s (cloudflared %s, checksum checked)...", p.Label, cloudflaredVersion)
	if err := installConnector(j, &c, p, id, token); err != nil {
		return id, err
	}
	j.Logf("%s is connected to Cloudflare. No router changes needed.", p.Label)
	return id, nil
}

// syncServerTunnelConfig publishes the hostnames of every project that a
// server's tunnel serves.
func syncServerTunnelConfig(c *Config, serverID string) error {
	p := findServer(c, serverID)
	if p == nil || p.TunnelID == "" {
		return nil
	}
	cf, err := cfFromConfig(c)
	if err != nil {
		return err
	}
	acc, err := cfAccount(c, cf)
	if err != nil {
		return err
	}
	var hosts []string
	for _, s := range c.Sites {
		if s.HAMode == "together" {
			continue // served by the project's own tunnel
		}
		for _, id := range siteServerIDs(s) {
			if id == serverID {
				hosts = append(hosts, siteHosts(s)...)
			}
		}
	}
	return cf.PutTunnelConfig(acc, p.TunnelID, tunnelIngress(serviceFor(p, c), hosts))
}

// ---------- project tunnels ("Together" mode) ----------

// ensureProjectTunnel creates the project's tunnel, publishes its ingress,
// starts a connector on every server and points the domain at it.
func ensureProjectTunnel(j *Job, slug string) error {
	c := snapshotConfig()
	s := findSite(&c, slug)
	if s == nil {
		return errors.New("unknown project")
	}
	cf, err := cfFromConfig(&c)
	if err != nil {
		return err
	}
	acc, err := cfAccount(&c, cf)
	if err != nil {
		return err
	}
	id, token := s.TunnelID, secret(s.TunnelToken)
	if id == "" {
		j.Logf("Creating the Cloudflare tunnel dilreet-%s...", slug)
		t, err := cf.CreateTunnel(acc, "dilreet-"+slug+"-"+randHex(2))
		if err != nil {
			return err
		}
		id, token = t.ID, t.Token
		if token == "" {
			if token, err = cf.TunnelToken(acc, id); err != nil {
				return err
			}
		}
		enc, err := protectString(token)
		if err != nil {
			return err
		}
		_ = withConfig(func(c *Config) error {
			if x := findSite(c, slug); x != nil {
				x.TunnelID, x.TunnelToken = id, enc
			}
			return nil
		})
	}
	hosts := siteHosts(s)
	if err := cf.PutTunnelConfig(acc, id, tunnelIngress("https://localhost:443", hosts)); err != nil {
		return err
	}
	var failed []string
	for _, sid := range siteServerIDs(s) {
		p := findServer(&c, sid)
		if p == nil {
			continue
		}
		if p.Kind == "thispc" {
			j.Logf("This PC is skipped in Together mode (it does not answer HTTPS on port 443). Use Primary + backup to include it.")
			continue
		}
		j.Logf("Starting a tunnel connector on %s...", p.Label)
		if err := installConnector(j, &c, p, id, token); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", p.Label, shortErr(err)))
		}
	}
	if s.Domain != "" && s.CFZoneID != "" {
		for _, h := range hosts {
			if _, err := cfPoint(cf, s.CFZoneID, h, "CNAME", tunnelTarget(id)); err != nil {
				return fmt.Errorf("point %s at the tunnel: %w", h, err)
			}
		}
		j.Logf("%s now points at the tunnel (orange cloud). Cloudflare sends visitors to whichever server is up.", strings.Join(hosts, " and "))
	} else if s.Domain == "" {
		j.Logf("Connect a domain (on Cloudflare) to the project; Rover then points it at the tunnel.")
	}
	if len(failed) > 0 {
		return fmt.Errorf("the tunnel runs, but not on: %s", strings.Join(failed, "; "))
	}
	return nil
}

// tunnelStatus reports the live connectors of a tunnel.
func tunnelStatus(c *Config, id string) (*CFTunnel, error) {
	if id == "" {
		return nil, nil
	}
	cf, err := cfFromConfig(c)
	if err != nil {
		return nil, err
	}
	acc, err := cfAccount(c, cf)
	if err != nil {
		return nil, err
	}
	t, err := cf.Tunnel(acc, id)
	if err != nil {
		return nil, err
	}
	t.Token = ""
	return &t, nil
}

// ensureHA sets up what the project's mode needs after a server change.
func ensureHA(j *Job, c *Config, s *Site) (string, error) {
	switch s.HAMode {
	case "together":
		if c.CloudflareToken == "" {
			return "", errors.New("Together mode needs the domain on Cloudflare (free). Connect Cloudflare in Settings, then click Turn on")
		}
		return "Together mode: every server answers visitors through one Cloudflare tunnel.", ensureProjectTunnel(j, s.Slug)
	case "backup":
		for _, id := range siteServerIDs(s) {
			if p := findServer(c, id); p != nil && p.Reach == "tunnel" {
				_ = syncServerTunnelConfig(c, id)
			}
		}
		if c.CloudflareToken == "" || s.DNSMode != "cloudflare" {
			return "Primary + backup: Rover checks the servers, but switching needs the domain on Cloudflare. Until then it tells you when to switch and shows the steps.", nil
		}
		return "Primary + backup: Rover checks every server each minute and switches the domain's DNS at Cloudflare if the main one goes down.", nil
	}
	return "", nil
}

func osAsset() string {
	if runtime.GOOS == "windows" {
		return "windows-amd64.exe"
	}
	return "linux-" + runtime.GOARCH
}

func jsonString(v any) string { b, _ := json.Marshal(v); return string(b) }
