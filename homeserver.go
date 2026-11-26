package main

// Rover 0.7: home-made servers.
//
// A Linux machine at home (old PC, mini PC, Raspberry Pi) is added like any
// server: Rover logs in over SSH once with the password, installs its own
// key, and from then on works without the password. Machines with little
// memory (or 32-bit ARM) get "lite" mode: plain nginx on the machine itself,
// no Coolify. Home internet usually has no reachable public IP (CGNAT) and a
// changing address, so Rover connects the machine to Cloudflare with a
// tunnel (outgoing connection only, no router changes). Port forwarding plus
// dynamic DNS is offered as a guided alternative.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// liteOpts: a site served by the host's own nginx.
type liteOpts struct {
	slug  string
	hosts []string
}

// liteKey turns a project id into a safe nginx variable fragment.
func liteKey(slug string) string {
	var b strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

var liteVarRe = regexp.MustCompile(`([$<{])hp_`)

// liteConf converts the container nginx config of a site into one that the
// host's nginx can load next to other sites: its own names instead of
// default_server, HTTPS on 443 with the machine's self-signed certificate
// (for Cloudflare and tunnels), the site's folder as root, and variable
// names unique per site (geo/map names are global in nginx).
func liteConf(conf, slug, base string, hosts []string) string {
	names := "_"
	if len(hosts) > 0 {
		names = strings.Join(hosts, " ")
	}
	conf = strings.Replace(conf, "    listen 80 default_server;\n    server_name _;\n",
		"    listen 80;\n    listen [::]:80;\n    listen 443 ssl;\n    listen [::]:443 ssl;\n"+
			"    ssl_certificate /etc/ssl/certs/ssl-cert-snakeoil.pem;\n    ssl_certificate_key /etc/ssl/private/ssl-cert-snakeoil.key;\n"+
			"    server_name "+names+";\n", 1)
	conf = strings.Replace(conf, "    root /usr/share/nginx/html;\n", "    root "+base+"/html;\n", 1)
	conf = strings.ReplaceAll(conf, "/etc/nginx/conf.d/.htpasswd", base+"/nginx/.htpasswd")
	conf = liteVarRe.ReplaceAllString(conf, "${1}hp"+liteKey(slug)+"_")
	return conf
}

// liteShellFuncs are hp_conf_install/hp_reload for host nginx. A release's
// config may still be in container form (made before the server was lite),
// so it is converted with sed on the way in.
func liteShellFuncs(slug string) string {
	key := liteKey(slug)
	return fmt.Sprintf(`
HP_RELOAD=""
hp_container() { :; }
hp_conf_install() {
  CD="$BASE/nginx"
  mkdir -p "$CD"
  if [ -f "$CD/default.conf" ]; then cp -f "$CD/default.conf" "$CD/.prev.conf.bak"; else rm -f "$CD/.prev.conf.bak"; fi
  if grep -q 'listen 80 default_server' "$1"; then
    sed -e 's#listen 80 default_server;#listen 80; listen 443 ssl; ssl_certificate /etc/ssl/certs/ssl-cert-snakeoil.pem; ssl_certificate_key /etc/ssl/private/ssl-cert-snakeoil.key;#' \
        -e "s#server_name _;#server_name $(cat "$CD/.hosts" 2>/dev/null || echo _);#" \
        -e "s#root /usr/share/nginx/html;#root $BASE/html;#" \
        -e "s#/etc/nginx/conf.d/.htpasswd#$BASE/nginx/.htpasswd#" \
        -e 's#\([$<{]\)hp_#\1hp%[1]s_#g' "$1" > "$CD/.new.tmp"
  else
    cp -f "$1" "$CD/.new.tmp"
  fi
  chmod 644 "$CD/.new.tmp" && mv -f "$CD/.new.tmp" "$CD/default.conf"
  INC="${HP_NGINX_CONFD:-/etc/nginx/conf.d}/dilreet-%[2]s.conf"
  [ -f "$INC" ] || printf 'include %%s/nginx/default.conf;\n' "$BASE" > "$INC"
  if ! OUT=$(nginx -t 2>&1); then
    echo "nginx rejected the generated config:" >&2; echo "$OUT" >&2
    if [ -f "$CD/.prev.conf.bak" ]; then mv -f "$CD/.prev.conf.bak" "$CD/default.conf"; fi
    return 1
  fi
  rm -f "$CD/.prev.conf.bak"
  echo "nginx config test passed"
}
hp_reload() {
  nginx -s reload 2>/dev/null || systemctl reload nginx 2>/dev/null || service nginx reload 2>/dev/null || echo "WARNING: nginx reload failed; run: sudo systemctl restart nginx"
}
`, key, slug)
}

// liteHosts: the names a lite server answers to for a project.
func liteHosts(s *Site, p *ServerProfile) []string {
	var out []string
	seen := map[string]bool{}
	add := func(h string) {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	if s != nil {
		for _, h := range siteHosts(s) {
			add(h)
		}
		if p != nil && p.ServerIP != "" {
			add(sslipHost(s.Slug, p.ServerIP))
		}
		add(s.Slug + ".local")
	}
	return out
}

// liteSiteScript creates a site folder on a lite server and enables it.
func liteSiteScript(base, slug string, hosts []string) string {
	delim := "HPCONF_" + randHex(8)
	def := strings.TrimRight(liteConf(defaultNginxConf(), slug, base, hosts), "\n")
	return fmt.Sprintf(`set -e
BASE=%[1]s
%[5]s
mkdir -p "$BASE/releases/initial" "$BASE/nginx" "$BASE/nginx-releases"
printf '%%s' %[6]s > "$BASE/nginx/.hosts"
cat > "$BASE/nginx-releases/_default.conf" <<'%[4]s'
%[3]s
%[4]s
if [ ! -e "$BASE/html" ]; then
  cat > "$BASE/releases/initial/index.html" <<'HPEOF'
%[2]s
HPEOF
  ln -sfn releases/initial "$BASE/html"
fi
chmod -R a+rX "$BASE"
[ -f "$BASE/nginx/default.conf" ] && grep -q 'ssl-cert-snakeoil' "$BASE/nginx/default.conf" || hp_conf_install "$BASE/nginx-releases/_default.conf"
hp_reload
echo ok`, shellQuote(base), placeholderHTML, def, delim, liteShellFuncs(slug), shellQuote(strings.Join(hosts, " ")))
}

// liteHostsScript updates the names of a site on a lite server (after a
// domain is connected) in the live config and every saved release config.
func liteHostsScript(base, slug string, hosts []string) string {
	names := strings.Join(hosts, " ")
	return fmt.Sprintf(`set -e
BASE=%[1]s
%[3]s
printf '%%s' %[2]s > "$BASE/nginx/.hosts"
for f in "$BASE"/nginx-releases/*.conf; do [ -f "$f" ] && grep -q ssl-cert-snakeoil "$f" && sed -i "s#^\(\s*server_name\) .*;#\1 $(cat "$BASE/nginx/.hosts");#" "$f"; done
cp "$BASE/nginx/default.conf" "$BASE/nginx/.cur.conf"
sed -i "s#^\(\s*server_name\) .*;#\1 $(cat "$BASE/nginx/.hosts");#" "$BASE/nginx/.cur.conf"
hp_conf_install "$BASE/nginx/.cur.conf"
rm -f "$BASE/nginx/.cur.conf"
hp_reload
echo ok`, shellQuote(base), shellQuote(names), liteShellFuncs(slug))
}

// liteEngineScript installs plain nginx (lite mode). HP_DRY=1 prints the
// commands instead of running them (tests).
const liteEngineScript = `set -eu
run() { if [ -n "${HP_DRY:-}" ]; then echo "+ $*"; else "$@"; fi; }
if command -v apt-get >/dev/null 2>&1 || [ -n "${HP_DRY:-}" ]; then
  echo "Installing nginx (lightweight mode)..."
  run env DEBIAN_FRONTEND=noninteractive apt-get update -q
  run env DEBIAN_FRONTEND=noninteractive apt-get install -y -q nginx ssl-cert curl ca-certificates
elif command -v dnf >/dev/null 2>&1; then
  run dnf install -y nginx curl openssl
  if [ ! -f /etc/ssl/certs/ssl-cert-snakeoil.pem ]; then
    run mkdir -p /etc/ssl/private
    run openssl req -x509 -nodes -newkey rsa:2048 -days 3650 -subj /CN=dilreet-home -keyout /etc/ssl/private/ssl-cert-snakeoil.key -out /etc/ssl/certs/ssl-cert-snakeoil.pem
  fi
else
  echo "This Linux has neither apt nor dnf; install nginx yourself, then run setup again." >&2
  exit 1
fi
run mkdir -p /data/hostpanel/sites
run systemctl enable --now nginx
if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
  run ufw allow 80/tcp
  run ufw allow 443/tcp
fi
echo "nginx ready: $(nginx -v 2>&1 || echo dry-run)"
`

// chooseEngine picks Coolify or lite for a machine.
func chooseEngine(f ServerFacts) (engine, why string) {
	arch := strings.ToLower(f.Arch)
	switch {
	case strings.HasPrefix(arch, "armv7") || strings.HasPrefix(arch, "armv6") || arch == "armhf":
		return "lite", "32-bit ARM (Coolify needs 64-bit)"
	case f.RAMMB > 0 && f.RAMMB < 1800:
		return "lite", fmt.Sprintf("%d MB memory (Coolify needs about 2 GB)", f.RAMMB)
	case !f.Supported && !f.Coolify:
		return "lite", fmt.Sprintf("%s is not supported by Coolify's installer", f.OSName)
	}
	return "coolify", "enough memory for the full hosting engine"
}

// HomeSetupOptions for the "My own server (at home)" wizard.
type HomeSetupOptions struct {
	Engine string `json:"engine"` // auto | lite | coolify
	Reach  string `json:"reach"`  // tunnel | portforward | lan
	SetupOptions
}

// homeSetupJob prepares the active server (a home machine) for hosting.
func homeSetupJob(j *Job, o HomeSetupOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Minute)
	defer cancel()
	c := snapshotConfig()
	p := findServer(&c, c.ActiveServer)
	if p == nil {
		return errors.New("add the machine first (Hosting accounts > My own server)")
	}
	j.Logf("Checking %s...", p.Label)
	f, err := serverFacts(ctx)
	if err != nil {
		return err
	}
	engine, why := chooseEngine(f)
	if o.Engine == "lite" || o.Engine == "coolify" {
		engine, why = o.Engine, "your choice"
	}
	j.Logf("%s: %s, %s, %d MB memory, %d GB free. Mode: %s (%s).", p.Label, f.OSName, f.Arch, f.RAMMB, f.DiskFreeG, map[string]string{"lite": "lightweight (nginx only)", "coolify": "full (Coolify)"}[engine], why)
	_ = withConfig(func(c *Config) error {
		if x := findServer(c, p.ID); x != nil {
			x.Kind, x.Engine, x.Arch, x.RAMMB = "home", map[string]string{"lite": "lite"}[engine], f.Arch, f.RAMMB
			if o.Reach == "tunnel" || o.Reach == "portforward" {
				x.Reach = o.Reach
			}
		}
		return nil
	})
	if engine == "coolify" {
		if err := setupServerJob(j, o.SetupOptions); err != nil {
			return err
		}
	} else {
		j.SetSteps("Checking the machine", "Installing nginx", "Connecting Rover", "Internet access", "Ready")
		j.Step(1)
		if o.RememberKey {
			if err := installManagedKey(ctx); err != nil {
				j.Logf("Note: %v", err)
			} else {
				j.Logf("Password-free login set up (Rover's own key; your password is not stored).")
			}
		}
		if err := runRemote(ctx, liteEngineScript, j); err != nil {
			return fmt.Errorf("installing nginx failed: %w", err)
		}
		j.Step(2)
	}
	if o.Reach == "tunnel" {
		j.Step(3)
		if _, err := ensureServerTunnel(j, p.ID); err != nil {
			j.Logf("The machine works on your home network, but the internet connection is not set up yet: %v", err)
			j.Logf("Fix: Settings > Cloudflare (add a token with Cloudflare Tunnel: Edit), then click Connect to the internet on this server.")
		}
	} else if o.Reach == "portforward" {
		j.Logf("Next: forward ports 80 and 443 on your router to %s. Rover shows the steps on the server page.", p.ServerIP)
	}
	j.Step(4)
	j.Logf("%s is ready. Create a project on it, or add it to a project as an extra server.", p.Label)
	return nil
}

// addHomeServer registers a machine reachable over SSH (normally on the home
// network). The password is used only by the connect step that follows and
// is never stored.
func addHomeServer(label, host string, port int, user string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", errors.New("enter the machine's address (for example 192.168.1.50 or raspberrypi.local)")
	}
	if strings.ContainsAny(host, " /\\@:") && net.ParseIP(host) == nil {
		return "", errors.New("enter just the address, without user@ or a port")
	}
	if port == 0 {
		port = 22
	}
	user = firstNonEmpty(strings.TrimSpace(user), "root")
	var id string
	err := withConfig(func(c *Config) error {
		for _, sp := range c.Servers {
			if strings.EqualFold(sp.ServerIP, host) {
				id = sp.ID
				return nil
			}
		}
		id = "home-" + randHex(4)
		c.Servers = append(c.Servers, &ServerProfile{
			ID: id, Label: firstNonEmpty(strings.TrimSpace(label), "Home server ("+host+")"), Provider: "home", Kind: "home",
			ServerIP: host, SSHPort: port, SSHUser: user, Added: time.Now().Format(time.RFC3339),
		})
		return nil
	})
	return id, err
}

// isPrivateHost reports whether host is a home-network address.
func isPrivateHost(host string) bool {
	if strings.HasSuffix(strings.ToLower(host), ".local") || strings.HasSuffix(strings.ToLower(host), ".lan") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || isCGNAT(ip))
}

func isCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127
}

// portForwardGuide is the plain-English alternative to a tunnel.
func portForwardGuide(p *ServerProfile) []string {
	ip := firstNonEmpty(p.ServerIP, "the machine's address")
	return []string{
		"Give the machine a fixed address on your network: in your router, find 'DHCP reservation' (or 'static lease') and reserve " + ip + " for it.",
		"Forward ports: in the router, find 'Port forwarding' (sometimes 'Virtual server' or 'NAT'). Add TCP 80 -> " + ip + ":80 and TCP 443 -> " + ip + ":443.",
		"Check that your internet has a real public address: compare the WAN address in the router with what whatismyip.com shows. If they differ (often 100.64.x.x), your provider uses CGNAT and port forwarding cannot work: use the Cloudflare tunnel instead.",
		"Your home address changes from time to time. Use a free dynamic-DNS name (DuckDNS, No-IP, or the one built into your router) and enter it below, or keep the domain on Cloudflare: Rover can update the record itself.",
		"Mobile data (phone hotspot) never accepts incoming connections. If you use it as a backup connection, only the Cloudflare tunnel keeps the site up while it is active.",
	}
}

// homeTradeoffs are shown before a home machine is used for a real site.
var homeTradeoffs = []string{
	"The machine must stay on and awake. If it sleeps, restarts for updates or loses power, the site goes down (unless another server is a backup).",
	"Your upload speed is the site's speed. A 10 Mbit/s upload serves a few visitors at once well; large images and videos fill it fast.",
	"Power cuts and internet outages take the site down. A small UPS keeps the router and machine running through short cuts.",
	"Best use: as a backup next to a rented server, for small personal sites, or for testing. Use a rented server for important sites.",
}

// probeLocalHTTP is a tiny helper used by the This PC checks.
func probeLocalHTTP(url string) error {
	cl := &http.Client{Timeout: 3 * time.Second}
	resp, err := cl.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
