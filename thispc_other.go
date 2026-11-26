//go:build !windows

package main

import (
	"net"
	"strings"
	"sync/atomic"
)

var keepAwake atomic.Bool

// Keeping the machine awake and starting with the OS are Windows features;
// elsewhere the settings are remembered but do nothing.
func setKeepAwake(on bool) { keepAwake.Store(on) }
func keepAwakeOn() bool    { return keepAwake.Load() }
func setAutostart(bool) error {
	return nil
}
func autostartOn() bool { return false }

func powerStatus() map[string]any { return map[string]any{"known": false} }

var listAdapters = func() []netAdapter {
	ifs, _ := net.Interfaces()
	var out []netAdapter
	for _, i := range ifs {
		if i.Flags&net.FlagLoopback != 0 {
			continue
		}
		a := netAdapter{Name: i.Name, Kind: "other", Up: i.Flags&net.FlagUp != 0, Metric: 100}
		n := strings.ToLower(i.Name)
		switch {
		case strings.HasPrefix(n, "wl"):
			a.Kind = "wifi"
		case strings.HasPrefix(n, "en") || strings.HasPrefix(n, "eth"):
			a.Kind = "ethernet"
		case strings.HasPrefix(n, "usb") || strings.HasPrefix(n, "rndis"):
			a.Kind = "usb-tether"
		case strings.HasPrefix(n, "wwan") || strings.HasPrefix(n, "ww"):
			a.Kind = "mobile"
		}
		addrs, _ := i.Addrs()
		for _, ad := range addrs {
			if ipn, ok := ad.(*net.IPNet); ok && ipn.IP.To4() != nil {
				a.IPv4 = append(a.IPv4, ipn.IP.String())
			}
		}
		a.Gateway = a.Up && len(a.IPv4) > 0
		out = append(out, a)
	}
	return out
}
