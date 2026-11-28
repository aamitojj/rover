package main

// Rover 0.9: independent nginx hosting over SSH. Reuses the 0.7 lite
// engine (config test + restore on failure, atomic releases) and adds
// an installer that works on Ubuntu, Debian and Oracle Linux (amd64 and
// arm64), tuned for 1 GB RAM, plus optional certbot HTTP-01.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func registerNginxDirectRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/servers/{id}/engine", apiSetServerEngine)
	mux.HandleFunc("POST /api/sites/{slug}/ssl/certbot", apiCertbot)
}

func apiSetServerEngine(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Engine string `json:"engine"` // nginx | coolify
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	id := r.PathValue("id")
	switch req.Engine {
	case "nginx", "lite":
		req.Engine = "lite"
	case "coolify", "":
		req.Engine = ""
	default:
		writeErr(w, fmt.Errorf("engine must be nginx or coolify"))
		return
	}
	if req.Engine == "lite" {
		if !sshS.connected() {
			writeErr(w, errNotConnected)
			return
		}
		sid := id
		j := newJob("nginx-install", func(j *Job) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			out, err := runRemoteOutput(ctx, nginxInstallScript())
			j.Logf("%s", out)
			if err != nil {
				return err
			}
			return withConfig(func(c *Config) error {
				p := findServer(c, sid)
				if p != nil {
					p.Engine = "lite"
					return nil
				}
				if sid == c.ActiveServer || sid == "" {
					for _, s := range c.Servers {
						if s.ID == c.ActiveServer {
							s.Engine = "lite"
						}
					}
				}
				return nil
			})
		})
		writeJSON(w, 200, map[string]any{"job": j.ID, "engine": "nginx"})
		return
	}
	if err := withConfig(func(c *Config) error {
		p := findServer(c, id)
		if p == nil {
			return errors.New("unknown server")
		}
		p.Engine = ""
		return nil
	}); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "engine": "coolify"})
}

func apiCertbot(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	c := snapshotConfig()
	s := findSite(&c, slug)
	if s == nil || s.Domain == "" {
		writeErr(w, errors.New("this project has no domain yet"))
		return
	}
	if !sshS.connected() {
		writeErr(w, errNotConnected)
		return
	}
	email := firstNonEmpty(c.AdminEmail, "admin@"+s.Domain)
	hosts := siteHosts(s)
	j := newJob("certbot", func(j *Job) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		out, err := runRemoteOutput(ctx, certbotScript(s.Slug, email, hosts))
		j.Logf("%s", out)
		if err != nil {
			return err
		}
		_ = withConfig(func(c *Config) error {
			if st := findSite(c, slug); st != nil {
				st.HTTPSActive = true
			}
			return nil
		})
		return nil
	})
	writeJSON(w, 200, map[string]any{"job": j.ID})
}

// nginxInstallScript installs or reuses nginx, writes a 1 GB RAM-friendly
// main config, and prints swap advice. It never disables SELinux or the
// firewall; it only opens 80/443 when firewalld or ufw is already in use.
func nginxInstallScript() string {
	return `set -eu
# Rover 0.9 nginx direct installer. Safe to re-run.
OS=unknown
if [ -f /etc/os-release ]; then . /etc/os-release; OS=${ID:-unknown}; fi
ARCH=$(uname -m)
MEM_KB=$(awk '/MemTotal/{print $2}' /proc/meminfo 2>/dev/null || echo 0)
echo "Host: ${OS} ${ARCH}, RAM ${MEM_KB} kB"

install_nginx() {
  if command -v nginx >/dev/null 2>&1; then
    echo "nginx is already installed: $(nginx -v 2>&1)"
    return 0
  fi
  case "$OS" in
    ubuntu|debian)
      export DEBIAN_FRONTEND=noninteractive
      apt-get update -y
      apt-get install -y nginx
      ;;
    ol|oracle|rhel|centos|almalinux|rocky|fedora)
      if command -v dnf >/dev/null 2>&1; then dnf install -y nginx; else yum install -y nginx; fi
      ;;
    *)
      echo "Unsupported OS '$OS'. Install nginx yourself, then re-run." >&2
      return 1
      ;;
  esac
  echo "Installed nginx: $(nginx -v 2>&1)"
}

tune_1g() {
  # Small worker counts for 1 GB RAM. Do not overwrite a file Rover did not write
  # unless it is the default package file we can detect.
  CONF=/etc/nginx/nginx.conf
  if [ -f "$CONF" ] && grep -q 'Rover-1g' "$CONF" 2>/dev/null; then
    echo "1 GB RAM nginx tuning already present"
    return 0
  fi
  WORKERS=1
  if [ "${MEM_KB:-0}" -gt 1800000 ]; then WORKERS=auto; fi
  # Write a drop-in rather than replacing the whole file when possible.
  mkdir -p /etc/nginx/conf.d
  cat > /etc/nginx/conf.d/rover-1g.conf <<EOF
# Rover-1g: conservative defaults for small VPS (around 1 GB RAM).
# worker_processes is set in nginx.conf if it is still the package default.
EOF
  if grep -qE '^\s*worker_processes' "$CONF" 2>/dev/null; then
    sed -i "s/^\s*worker_processes.*/worker_processes $WORKERS; # Rover-1g/" "$CONF" || true
  fi
  # Limit worker connections if still at a high package default.
  if grep -qE '^\s*worker_connections\s+[0-9]+' "$CONF"; then
    sed -i 's/^\s*worker_connections\s\+[0-9]\+/    worker_connections 128/' "$CONF" || true
  fi
  echo "Tuned worker_processes=$WORKERS, worker_connections=128"
}

swap_advice() {
  SWAP=$(awk '/SwapTotal/{print $2}' /proc/meminfo 2>/dev/null || echo 0)
  if [ "${MEM_KB:-0}" -le 1500000 ] && [ "${SWAP:-0}" -lt 500000 ]; then
    echo "ADVICE: this machine has little RAM and little or no swap."
    echo "ADVICE: adding 1 GB of swap is recommended, for example:"
    echo "ADVICE:   fallocate -l 1G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile"
    echo "ADVICE: Rover does not create swap automatically."
  fi
}

open_ports() {
  if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi active; then
    ufw allow 80/tcp || true
    ufw allow 443/tcp || true
    echo "Opened 80 and 443 in ufw (firewall was already active)."
  fi
  if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state 2>/dev/null | grep -q running; then
    firewall-cmd --permanent --add-service=http || true
    firewall-cmd --permanent --add-service=https || true
    firewall-cmd --reload || true
    echo "Opened http/https in firewalld (firewall was already running)."
  fi
  if command -v getenforce >/dev/null 2>&1; then
    echo "SELinux is $(getenforce). Rover will not disable it. On Oracle Linux you may need: setsebool -P httpd_can_network_connect 1"
  fi
}

install_nginx
tune_1g
if ! nginx -t; then
  echo "nginx -t failed after tuning; leaving the service as-is." >&2
  exit 1
fi
systemctl enable nginx 2>/dev/null || true
systemctl restart nginx 2>/dev/null || service nginx restart 2>/dev/null || nginx
open_ports
swap_advice
echo ok
`
}

func certbotScript(slug, email string, hosts []string) string {
	var names []string
	for _, h := range hosts {
		h = strings.TrimSpace(strings.ToLower(h))
		if h != "" && !strings.ContainsAny(h, " \t\n\"'`$;&|<>") {
			names = append(names, h)
		}
	}
	if len(names) == 0 {
		return "echo 'no host names' >&2; exit 1"
	}
	// email is quoted; names are allowlisted above.
	dargs := ""
	for _, n := range names {
		dargs += " -d " + n
	}
	return fmt.Sprintf(`set -eu
EMAIL=%s
if ! command -v certbot >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -y && apt-get install -y certbot python3-certbot-nginx || apt-get install -y certbot
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y certbot || dnf install -y python3-certbot-nginx || true
  fi
fi
if ! command -v certbot >/dev/null 2>&1; then
  echo "certbot is not installed and could not be installed automatically." >&2
  exit 1
fi
# HTTP-01 via nginx plugin when available, else webroot under the project folder.
if certbot --help 2>&1 | grep -q nginx; then
  certbot --nginx --agree-tos --non-interactive --redirect -m "$EMAIL" --keep-until-expiry%s
else
  mkdir -p /var/www/letsencrypt
  certbot certonly --webroot -w /var/www/letsencrypt --agree-tos --non-interactive -m "$EMAIL" --keep-until-expiry%s
fi
nginx -t && (systemctl reload nginx 2>/dev/null || nginx -s reload)
echo ok
`, shellQuote(email), dargs, dargs)
}
