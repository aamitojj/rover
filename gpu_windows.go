//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// platformGPUs reads the display adapters from the registry (no WMI, no
// extra processes) and checks for the Vulkan loader.
func platformGPUs() ([]GPUInfo, bool) {
	const class = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`
	var out []GPUInfo
	for i := 0; i < 16; i++ {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, class+`\`+fmt.Sprintf("%04d", i), registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		name, _, _ := k.GetStringValue("DriverDesc")
		dev, _, _ := k.GetStringValue("MatchingDeviceId")
		var vram uint64
		if v, _, err := k.GetIntegerValue("HardwareInformation.qwMemorySize"); err == nil {
			vram = v
		} else if b, _, err := k.GetBinaryValue("HardwareInformation.qwMemorySize"); err == nil && len(b) >= 8 {
			vram = binary.LittleEndian.Uint64(b)
		} else if v, _, err := k.GetIntegerValue("HardwareInformation.MemorySize"); err == nil {
			vram = v
		} else if b, _, err := k.GetBinaryValue("HardwareInformation.MemorySize"); err == nil && len(b) >= 4 {
			vram = uint64(binary.LittleEndian.Uint32(b))
		}
		k.Close()
		if name == "" {
			continue
		}
		out = append(out, GPUInfo{Name: name, Vendor: vendorOf(dev, name), VRAM: vram})
	}
	sys := os.Getenv("SystemRoot")
	if sys == "" {
		sys = `C:\Windows`
	}
	_, err := os.Stat(filepath.Join(sys, "System32", "vulkan-1.dll"))
	return out, err == nil
}
