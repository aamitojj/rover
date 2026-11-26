package main

// Rover 0.8: graphics card detection for the offline brain. A dedicated
// GPU with enough memory runs the model with llama.cpp's Vulkan build (one
// build for NVIDIA, AMD and Intel); llama-server's --fit then places as many
// layers on the card as fit and keeps the rest in RAM. Without a usable GPU
// the CPU build is used.

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type GPUInfo struct {
	Name      string `json:"name"`
	Vendor    string `json:"vendor"` // nvidia | amd | intel | other
	VRAM      uint64 `json:"vram"`   // bytes of dedicated memory (0 = unknown)
	Dedicated bool   `json:"dedicated"`
}

type HWInfo struct {
	GPUs      []GPUInfo `json:"gpus"`
	Vulkan    bool      `json:"vulkan"` // the Vulkan loader is installed (comes with the graphics driver)
	RAM       uint64    `json:"ram"`
	Cores     int       `json:"cores"`
	Simulated bool      `json:"simulated,omitempty"`
}

type BrainRec struct {
	Model string `json:"model"`
	GPU   bool   `json:"gpu"`
	Why   string `json:"why"`
	Speed string `json:"speed"`
	Card  string `json:"card,omitempty"`
}

var (
	hwOnce sync.Once
	hwInfo HWInfo
	// detectGPUs is replaced in tests.
	detectGPUs = func() ([]GPUInfo, bool) { return platformGPUs() }
)

func vendorOf(id, name string) string {
	id, name = strings.ToLower(id), strings.ToLower(name)
	switch {
	case strings.Contains(id, "10de") || strings.Contains(name, "nvidia") || strings.Contains(name, "geforce") || strings.Contains(name, "quadro"):
		return "nvidia"
	case strings.Contains(id, "1002") || strings.Contains(name, "amd") || strings.Contains(name, "radeon"):
		return "amd"
	case strings.Contains(id, "8086") || strings.Contains(name, "intel"):
		return "intel"
	}
	return "other"
}

// isDedicated tells discrete cards from integrated graphics (which share RAM
// and gain little).
func isDedicated(g GPUInfo) bool {
	n := strings.ToLower(g.Name)
	if strings.Contains(n, "basic display") || strings.Contains(n, "virtual") || strings.Contains(n, "remote") || strings.Contains(n, "parsec") || strings.Contains(n, "citrix") {
		return false
	}
	switch g.Vendor {
	case "nvidia":
		return true
	case "amd":
		return g.VRAM >= 2<<30 && !strings.Contains(n, "radeon(tm) graphics") && !strings.Contains(n, "vega 8") && !strings.Contains(n, "vega 7")
	case "intel":
		return strings.Contains(n, "arc") && !strings.Contains(n, "arc(tm) graphics")
	}
	return false
}

func hardware(refresh bool) HWInfo {
	if refresh {
		hwOnce = sync.Once{}
	}
	hwOnce.Do(func() {
		gpus, vk := detectGPUs()
		sim := false
		if f := envOr("FAKE_GPU"); f != "" { // demos and tests: "name|MiB"
			name, mib, _ := strings.Cut(f, "|")
			n, _ := strconv.Atoi(mib)
			gpus, vk, sim = []GPUInfo{{Name: name, Vendor: vendorOf("", name), VRAM: uint64(n) << 20}}, true, true
		}
		for i := range gpus {
			gpus[i].Dedicated = isDedicated(gpus[i])
		}
		sort.SliceStable(gpus, func(i, j int) bool {
			if gpus[i].Dedicated != gpus[j].Dedicated {
				return gpus[i].Dedicated
			}
			return gpus[i].VRAM > gpus[j].VRAM
		})
		hwInfo = HWInfo{GPUs: gpus, Vulkan: vk, RAM: totalRAM(), Cores: runtime.NumCPU(), Simulated: sim}
	})
	return hwInfo
}

func gib(b uint64) string {
	g := float64(b) / float64(1<<30)
	if g >= 9.5 {
		return fmt.Sprintf("%.0f GB", g)
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", g), ".0") + " GB"
}

// recommendFor picks the model and engine for a PC.
func recommendFor(hw HWInfo) BrainRec {
	var best *GPUInfo
	for i := range hw.GPUs {
		if hw.GPUs[i].Dedicated {
			best = &hw.GPUs[i]
			break
		}
	}
	if best != nil && hw.Vulkan {
		card := best.Name
		switch {
		case best.VRAM >= 3500<<20:
			return BrainRec{Model: "qwen3-4b", GPU: true, Card: card,
				Why:   fmt.Sprintf("Your %s (%s) runs the Standard model on the graphics card. Parts that don't fit stay in RAM automatically.", card, gib(best.VRAM)),
				Speed: "about 15-30 words/s"}
		case best.VRAM >= 2<<30:
			return BrainRec{Model: "qwen3-1.7b", GPU: true, Card: card,
				Why:   fmt.Sprintf("Your %s (%s) runs the Light model on the graphics card.", card, gib(best.VRAM)),
				Speed: "about 30-50 words/s"}
		}
	}
	why := ""
	if best != nil && !hw.Vulkan {
		why = "Your graphics card's driver has no Vulkan support (update the driver to use the card). "
	}
	if hw.RAM >= 11<<30 && hw.Cores >= 4 {
		return BrainRec{Model: "qwen3-4b", Why: why + fmt.Sprintf("This PC has %s of memory: enough for the Standard model on the processor.", gib(hw.RAM)), Speed: "about 8-20 words/s"}
	}
	return BrainRec{Model: "qwen3-1.7b", Why: why + fmt.Sprintf("This PC has %s of memory, so the Light model keeps it responsive.", gib(hw.RAM)), Speed: "about 15-35 words/s"}
}
