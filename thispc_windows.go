//go:build windows

package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	kernel32pc                  = windows.NewLazySystemDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32pc.NewProc("SetThreadExecutionState")
	procGetSystemPowerStatus    = kernel32pc.NewProc("GetSystemPowerStatus")
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
	esAwayMode       = 0x00000040
)

// keep-awake: SetThreadExecutionState applies to the calling thread, so a
// dedicated locked thread holds the request until it is turned off.
var awake = struct {
	sync.Mutex
	stop chan struct{}
}{}

func setKeepAwake(on bool) {
	awake.Lock()
	defer awake.Unlock()
	if on == (awake.stop != nil) {
		return
	}
	if !on {
		close(awake.stop)
		awake.stop = nil
		return
	}
	stop := make(chan struct{})
	awake.stop = stop
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		procSetThreadExecutionState.Call(uintptr(esContinuous | esSystemRequired | esAwayMode))
		<-stop
		procSetThreadExecutionState.Call(uintptr(esContinuous))
	}()
}

func keepAwakeOn() bool {
	awake.Lock()
	defer awake.Unlock()
	return awake.stop != nil
}

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// setAutostart starts Rover in the background when the user signs in
// (current user only, no admin rights).
func setAutostart(on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if _, _, err := k.GetStringValue("Dilreet"); err == nil {
		_ = k.DeleteValue("Dilreet") // the entry from before the rename
	}
	if !on {
		if _, _, err := k.GetStringValue("Rover"); err == nil {
			return k.DeleteValue("Rover")
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue("Rover", `"`+exe+`" -browser -no-browser -autostart`)
}

func autostartOn() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue("Rover")
	if err != nil {
		v, _, err = k.GetStringValue("Dilreet")
	}
	return err == nil && strings.Contains(v, "-autostart")
}

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

func powerStatus() map[string]any {
	var s systemPowerStatus
	r, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&s)))
	if r == 0 {
		return map[string]any{"known": false}
	}
	out := map[string]any{"known": true, "on_ac": s.ACLineStatus == 1, "battery": s.BatteryFlag != 128 && s.BatteryFlag != 255}
	if s.BatteryLifePercent <= 100 {
		out["percent"] = int(s.BatteryLifePercent)
	}
	out["saver"] = s.SystemStatusFlag == 1
	return out
}

// listAdapters classifies network adapters (GetAdaptersAddresses):
// IfType 71 is Wi-Fi, 243/244 mobile broadband, and "Remote NDIS" adapters
// are phones shared over USB.
var listAdapters = func() []netAdapter {
	size := uint32(15000)
	var buf []byte
	var first *windows.IpAdapterAddresses
	for i := 0; i < 3; i++ {
		buf = make([]byte, size)
		first = (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(syscall.AF_INET, windows.GAA_FLAG_INCLUDE_GATEWAYS|windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_DNS_SERVER, 0, first, &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return nil
		}
		first = nil
	}
	ssid := wifiSSID()
	var out []netAdapter
	for a := first; a != nil; a = a.Next {
		if a.IfType == 24 { // loopback
			continue
		}
		name := windows.UTF16PtrToString(a.FriendlyName)
		desc := windows.UTF16PtrToString(a.Description)
		n := netAdapter{Name: name, Kind: "other", Up: a.OperStatus == windows.IfOperStatusUp, Gateway: a.FirstGatewayAddress != nil, Metric: int(a.Ipv4Metric)}
		ld := strings.ToLower(desc)
		switch {
		case strings.Contains(ld, "remote ndis") || strings.Contains(ld, "rndis") || strings.Contains(ld, "usb ethernet") && strings.Contains(ld, "phone"):
			n.Kind = "usb-tether"
		case a.IfType == 71:
			n.Kind, n.SSID = "wifi", ssid
		case a.IfType == 243 || a.IfType == 244:
			n.Kind = "mobile"
		case a.IfType == 6:
			n.Kind = "ethernet"
		}
		if strings.Contains(ld, "virtual") || strings.Contains(ld, "hyper-v") || strings.Contains(ld, "vpn") || strings.Contains(ld, "wan miniport") {
			if !n.Gateway {
				continue
			}
		}
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			if ip := u.Address.IP(); ip != nil && ip.To4() != nil {
				n.IPv4 = append(n.IPv4, ip.String())
			}
		}
		out = append(out, n)
	}
	return out
}

// wifiSSID reads the connected Wi-Fi name (netsh, no admin needed).
func wifiSSID() string {
	cmd := exec.Command("netsh", "wlan", "show", "interfaces")
	hideConsole(cmd)
	b, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(l, ":")
		if ok && strings.TrimSpace(k) == "SSID" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
