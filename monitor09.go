package main

// Rover 0.9 server monitoring: CPU, RAM, disk, swap and network via /proc
// over SSH, plus uptime, site availability, SSL expiry and service health.
// Alerts go to the in-app notice list, optionally a Windows notification,
// and optionally an email or webhook the user configured.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AlertSettings struct {
	InApp    bool   `json:"in_app"`
	Windows  bool   `json:"windows"`
	Email    string `json:"email,omitempty"`
	Webhook  string `json:"webhook,omitempty"`
	CPU      int    `json:"cpu,omitempty"` // percent, 0 = off
	RAM      int    `json:"ram,omitempty"`
	Disk     int    `json:"disk,omitempty"`
	SSLDays  int    `json:"ssl_days,omitempty"`
	Cooldown int    `json:"cooldown_min,omitempty"`
}

type ServerMetrics struct {
	Server    string  `json:"server"`
	Label     string  `json:"label"`
	Collected string  `json:"collected"`
	UptimeSec int64   `json:"uptime_sec"`
	CPU       float64 `json:"cpu_pct"`
	RAMUsed   uint64  `json:"ram_used"`
	RAMTotal  uint64  `json:"ram_total"`
	SwapUsed  uint64  `json:"swap_used"`
	SwapTotal uint64  `json:"swap_total"`
	DiskUsed  uint64  `json:"disk_used"`
	DiskTotal uint64  `json:"disk_total"`
	NetRx     uint64  `json:"net_rx"`
	NetTx     uint64  `json:"net_tx"`
	Error     string  `json:"error,omitempty"`
}

var (
	metricsMu   sync.Mutex
	lastMetrics = map[string]ServerMetrics{}
	lastAlert   = map[string]time.Time{}
)

const metricsScript = `set -e
U=$(awk '{print int($1)}' /proc/uptime)
# CPU: two /proc/stat samples 0.4s apart.
cpu() { awk '/^cpu /{u=$2+$4; t=$2+$3+$4+$5+$6+$7+$8; print u, t}' /proc/stat; }
read u1 t1 <<EOF
$(cpu)
EOF
sleep 0.4
read u2 t2 <<EOF
$(cpu)
EOF
CPU=0
if [ "$t2" -gt "$t1" ]; then CPU=$(awk -v u1="$u1" -v t1="$t1" -v u2="$u2" -v t2="$t2" 'BEGIN{printf "%.1f", (u2-u1)*100/(t2-t1)}'); fi
awk -v u="$U" -v cpu="$CPU" '
  /MemTotal:/{mt=$2}
  /MemAvailable:/{ma=$2}
  /SwapTotal:/{st=$2}
  /SwapFree:/{sf=$2}
  END{
    # values are kB
    printf "HP_UP %s\nHP_CPU %s\nHP_RAM %d %d\nHP_SWAP %d %d\n", u, cpu, mt-ma, mt, st-sf, st
  }
' /proc/meminfo
df -Pk / | awk 'NR==2{printf "HP_DISK %s %s\n",$3,$2}'
awk '$1 ~ /^(eth|ens|enp|eno|wlan|wl)/{
  rx+=$2; tx+=$10
} END{printf "HP_NET %d %d\n", rx+0, tx+0}' /proc/net/dev
`

func parseMetrics(server, label, out string) ServerMetrics {
	m := ServerMetrics{Server: server, Label: label, Collected: time.Now().UTC().Format(time.RFC3339)}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "HP_UP":
			if len(f) > 1 {
				m.UptimeSec, _ = strconv.ParseInt(f[1], 10, 64)
			}
		case "HP_CPU":
			if len(f) > 1 {
				m.CPU, _ = strconv.ParseFloat(f[1], 64)
			}
		case "HP_RAM":
			if len(f) > 2 {
				m.RAMUsed, _ = strconv.ParseUint(f[1], 10, 64)
				m.RAMTotal, _ = strconv.ParseUint(f[2], 10, 64)
				m.RAMUsed *= 1024
				m.RAMTotal *= 1024
			}
		case "HP_SWAP":
			if len(f) > 2 {
				m.SwapUsed, _ = strconv.ParseUint(f[1], 10, 64)
				m.SwapTotal, _ = strconv.ParseUint(f[2], 10, 64)
				m.SwapUsed *= 1024
				m.SwapTotal *= 1024
			}
		case "HP_DISK":
			if len(f) > 2 {
				m.DiskUsed, _ = strconv.ParseUint(f[1], 10, 64)
				m.DiskTotal, _ = strconv.ParseUint(f[2], 10, 64)
				m.DiskUsed *= 1024
				m.DiskTotal *= 1024
			}
		case "HP_NET":
			if len(f) > 2 {
				m.NetRx, _ = strconv.ParseUint(f[1], 10, 64)
				m.NetTx, _ = strconv.ParseUint(f[2], 10, 64)
			}
		}
	}
	return m
}

func collectServerMetrics(ctx context.Context, id, label string) ServerMetrics {
	out, err := runRemoteOutput(ctx, metricsScript)
	m := parseMetrics(id, label, out)
	if err != nil {
		m.Error = err.Error()
	}
	metricsMu.Lock()
	lastMetrics[id] = m
	metricsMu.Unlock()
	maybeAlertMetrics(m)
	return m
}

func maybeAlertMetrics(m ServerMetrics) {
	c := snapshotConfig()
	a := c.Alerts
	if a == nil {
		return
	}
	pct := func(used, total uint64) int {
		if total == 0 {
			return 0
		}
		return int(used * 100 / total)
	}
	var msgs []string
	if a.CPU > 0 && m.CPU >= float64(a.CPU) {
		msgs = append(msgs, fmt.Sprintf("CPU on %s is %.0f%%", m.Label, m.CPU))
	}
	if a.RAM > 0 && pct(m.RAMUsed, m.RAMTotal) >= a.RAM {
		msgs = append(msgs, fmt.Sprintf("RAM on %s is %d%%", m.Label, pct(m.RAMUsed, m.RAMTotal)))
	}
	if a.Disk > 0 && pct(m.DiskUsed, m.DiskTotal) >= a.Disk {
		msgs = append(msgs, fmt.Sprintf("Disk on %s is %d%%", m.Label, pct(m.DiskUsed, m.DiskTotal)))
	}
	cd := a.Cooldown
	if cd <= 0 {
		cd = 60
	}
	for _, msg := range msgs {
		key := m.Server + "|" + msg
		metricsMu.Lock()
		if time.Since(lastAlert[key]) < time.Duration(cd)*time.Minute {
			metricsMu.Unlock()
			continue
		}
		lastAlert[key] = time.Now()
		metricsMu.Unlock()
		fireAlert(a, "warn", "", msg)
	}
}

func fireAlert(a *AlertSettings, level, site, text string) {
	text = redact(text, false)
	if a == nil || a.InApp {
		notify(level, site, text)
	}
	if a != nil && a.Windows {
		raiseDesktopAlert(text)
	}
	if a != nil && a.Webhook != "" {
		go postWebhook(a.Webhook, map[string]any{"level": level, "site": site, "text": text, "time": time.Now().UTC().Format(time.RFC3339)})
	}
}

func postWebhook(raw string, body map[string]any) {
	b, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", raw, strings.NewReader(string(b)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func registerMonitor09Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/monitor", apiMonitor)
	mux.HandleFunc("POST /api/monitor/alerts", apiSaveAlerts)
	mux.HandleFunc("POST /api/monitor/check", func(w http.ResponseWriter, r *http.Request) {
		if !sshS.connected() {
			writeErr(w, errNotConnected)
			return
		}
		c := snapshotConfig()
		id, label := c.ActiveServer, "active server"
		for _, p := range c.Servers {
			if p.ID == c.ActiveServer {
				label = firstNonEmpty(p.Label, p.ServerIP)
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		writeJSON(w, 200, collectServerMetrics(ctx, firstNonEmpty(id, "active"), label))
	})
}

func apiMonitor(w http.ResponseWriter, r *http.Request) {
	c := snapshotConfig()
	metricsMu.Lock()
	ms := make([]ServerMetrics, 0, len(lastMetrics))
	for _, m := range lastMetrics {
		ms = append(ms, m)
	}
	metricsMu.Unlock()
	writeJSON(w, 200, map[string]any{
		"metrics": ms,
		"alerts":  alertSettings(&c),
		"health":  healthSnapshot(),
	})
}

func alertSettings(c *Config) AlertSettings {
	a := AlertSettings{InApp: true, Windows: true, CPU: 90, RAM: 90, Disk: 90, SSLDays: 21, Cooldown: 60}
	if c.Alerts != nil {
		a = *c.Alerts
	}
	return a
}

func apiSaveAlerts(w http.ResponseWriter, r *http.Request) {
	var req AlertSettings
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Webhook != "" && !strings.HasPrefix(req.Webhook, "https://") && !strings.HasPrefix(req.Webhook, "http://127.0.0.1") && !strings.HasPrefix(req.Webhook, "http://localhost") {
		writeErr(w, fmt.Errorf("webhook URL must be https (or a local http address)"))
		return
	}
	if err := withConfig(func(c *Config) error { c.Alerts = &req; return nil }); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, req)
}

func monitorTick() {
	if !sshS.connected() {
		return
	}
	c := snapshotConfig()
	id, label := c.ActiveServer, "active server"
	for _, p := range c.Servers {
		if p.ID == c.ActiveServer {
			label = firstNonEmpty(p.Label, p.ServerIP)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	collectServerMetrics(ctx, firstNonEmpty(id, "active"), label)
}

func raiseDesktopAlert(text string) {
	// Reuse the existing window flash; a full balloon is Windows-only in desktop_windows.go.
	flashNativeWindow()
}
