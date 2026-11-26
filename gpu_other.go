//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// platformGPUs reads /sys/class/drm (Linux; elsewhere it finds nothing).
func platformGPUs() ([]GPUInfo, bool) {
	var out []GPUInfo
	cards, _ := filepath.Glob("/sys/class/drm/card[0-9]")
	for _, c := range cards {
		vb, err := os.ReadFile(filepath.Join(c, "device", "vendor"))
		if err != nil {
			continue
		}
		v := strings.TrimSpace(string(vb))
		g := GPUInfo{Name: "Graphics card " + v, Vendor: vendorOf(v, "")}
		if b, err := os.ReadFile(filepath.Join(c, "device", "mem_info_vram_total")); err == nil {
			g.VRAM, _ = strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
		}
		out = append(out, g)
	}
	vk := false
	for _, p := range []string{"/usr/lib/x86_64-linux-gnu/libvulkan.so.1", "/usr/lib64/libvulkan.so.1", "/usr/lib/libvulkan.so.1"} {
		if _, err := os.Stat(p); err == nil {
			vk = true
		}
	}
	return out, vk
}
