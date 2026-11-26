package main

// Jiv's optional offline brain: llama.cpp's llama-server plus a small
// Apache-2.0 GGUF model, downloaded on demand (sha256-verified, resumable)
// or imported from files the user already has, and run as a sidecar on
// 127.0.0.1 with a random port and API key.

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// JivSettings live in config.json (no secrets).
type JivSettings struct {
	Model     string `json:"model,omitempty"`      // installed brain model id ("" = none)
	ModelPath string `json:"model_path,omitempty"` // imported model file (else the brain folder)
	Runtime   string `json:"runtime,omitempty"`    // folder with llama-server(.exe); "" = brain folder
	BrainOff  bool   `json:"brain_off,omitempty"`  // keep the brain installed but don't use it
	NoAuto    bool   `json:"no_autostart,omitempty"`
	IdleMin   int    `json:"idle_min,omitempty"` // unload after N idle minutes (default 20)
	Lang      string `json:"lang,omitempty"`     // auto | en | hi
	NoNudges  bool   `json:"no_nudges,omitempty"`
	Mirror    string `json:"mirror,omitempty"` // optional https base URL hosting the same files (company mirror)
	Threads   int    `json:"threads,omitempty"`
	GPU       bool   `json:"gpu,omitempty"` // Vulkan build (any GPU vendor)
}

func defaultJivSettings() *JivSettings { return &JivSettings{Lang: "auto", IdleMin: 20} }

func jivSettings(c *Config) JivSettings {
	if c.Jiv == nil {
		return *defaultJivSettings()
	}
	return *c.Jiv
}

type BrainModel struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Short    string   `json:"short"`
	File     string   `json:"file"`
	URL      string   `json:"url"`
	SHA256   string   `json:"sha256"`
	Size     int64    `json:"size"`
	RAMGB    float64  `json:"ram_gb"`    // memory while loaded (8k context)
	MinPCGB  int      `json:"min_pc_gb"` // recommended total RAM of the PC
	License  string   `json:"license"`
	Source   string   `json:"source"`
	Notes    string   `json:"notes"`
	Args     []string `json:"-"`
	Speed    string   `json:"speed"`
	Language string   `json:"language"`
}

type brainRuntime struct {
	Name    string `json:"name"`
	File    string `json:"file"`
	URL     string `json:"url,omitempty"`
	SHA256  string `json:"sha256"`
	License string `json:"license"`
	Size    int64  `json:"size"`
}

const llamaBuild = "b12360"

var (
	brainModels = []BrainModel{
		{ID: "qwen3-4b", Name: "Standard - Qwen3 4B Instruct 2507", Short: "Standard", File: "Qwen3-4B-Instruct-2507-Q4_K_M.gguf",
			URL: "https://huggingface.co/unsloth/Qwen3-4B-Instruct-2507-GGUF/resolve/main/Qwen3-4B-Instruct-2507-Q4_K_M.gguf", SHA256: "3605803b982cb64aead44f6c1b2ae36e3acdb41d8e46c8a94c6533bc4c67e597",
			Size: 2497281120, RAMGB: 3.5, MinPCGB: 12, License: "Apache-2.0", Source: "Qwen team (Alibaba), GGUF by Unsloth",
			Notes: "Best answers and tool use for its size; understands Hindi.", Args: []string{"-rea", "off"}, Speed: "about 8-20 words/s on a modern laptop CPU", Language: "100+ languages incl. Hindi"},
		{ID: "qwen3-1.7b", Name: "Light - Qwen3 1.7B", Short: "Light", File: "Qwen3-1.7B-Q4_K_M.gguf",
			URL: "https://huggingface.co/unsloth/Qwen3-1.7B-GGUF/resolve/main/Qwen3-1.7B-Q4_K_M.gguf", SHA256: "b139949c5bd74937ad8ed8c8cf3d9ffb1e99c866c823204dc42c0d91fa181897",
			Size: 1107409472, RAMGB: 2, MinPCGB: 4, License: "Apache-2.0", Source: "Qwen team (Alibaba), GGUF by Unsloth",
			Notes: "For older or 4-8 GB laptops. Shorter, simpler answers.", Args: []string{"-rea", "off"}, Speed: "about 15-35 words/s on a laptop CPU", Language: "100+ languages incl. Hindi"},
	}
	brainRuntimes = map[string]brainRuntime{
		"cpu":    {Name: "llama.cpp " + llamaBuild + " (CPU)", File: "llama-" + llamaBuild + "-bin-win-cpu-x64.zip", SHA256: "eae1726047e0eaa707b1302929d196c999b96c2f243b21aea6e93bcebacc4e17", Size: 25097627, License: "MIT"},
		"vulkan": {Name: "llama.cpp " + llamaBuild + " (Vulkan GPU)", File: "llama-" + llamaBuild + "-bin-win-vulkan-x64.zip", SHA256: "42a6f9df608b99c1c83fcaf77284a516e3f167b32a4690f4e4e364c377547055", Size: 38090978, License: "MIT"},
	}
	llamaReleaseBase = "https://github.com/ggml-org/llama.cpp/releases/download/" + llamaBuild + "/"
)

func brainModelByID(id string) (BrainModel, bool) {
	for _, m := range brainModels {
		if m.ID == id {
			return m, true
		}
	}
	return BrainModel{}, false
}

// brainDir is in %LOCALAPPDATA% (not the roaming profile): models are big.
func brainDir() (string, error) {
	if d := envOr("BRAIN_DIR"); d != "" {
		return d, os.MkdirAll(d, 0o700)
	}
	base, err := localDataDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, "Brain")
	return d, os.MkdirAll(d, 0o700)
}

func serverExeName() string {
	if runtime.GOOS == "windows" {
		return "llama-server.exe"
	}
	return "llama-server"
}

// brainPaths returns the runtime executable and model file in use.
func brainPaths(c *Config) (exe, model string, m BrainModel, ok bool) {
	js := jivSettings(c)
	if js.Model == "" {
		return "", "", m, false
	}
	m, known := brainModelByID(js.Model)
	if !known {
		m = BrainModel{ID: js.Model, Name: "Your model (" + filepath.Base(js.ModelPath) + ")", Short: "Custom", Args: nil}
	}
	dir, err := brainDir()
	if err != nil {
		return "", "", m, false
	}
	model = js.ModelPath
	if model == "" {
		model = filepath.Join(dir, m.File)
	}
	rt := js.Runtime
	if rt == "" {
		rt = runtimeDir(dir, js.GPU)
	}
	exe = filepath.Join(rt, serverExeName())
	if fileExists(exe) && fileExists(model) {
		return exe, model, m, true
	}
	return exe, model, m, false
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// runtimeDir is the engine folder: the Vulkan build when the GPU is on (it
// also runs on the CPU), else the CPU build; whichever is installed wins.
func runtimeDir(dir string, gpu bool) string {
	cpu, vk := filepath.Join(dir, "runtime-"+llamaBuild), filepath.Join(dir, "runtime-"+llamaBuild+"-vulkan")
	has := func(d string) bool { return fileExists(filepath.Join(d, serverExeName())) }
	switch {
	case gpu && has(vk):
		return vk
	case has(cpu):
		return cpu
	case has(vk):
		return vk
	}
	if gpu {
		return vk
	}
	return cpu
}

func runtimeDirFor(dir, kind string) string {
	if kind == "vulkan" {
		return filepath.Join(dir, "runtime-"+llamaBuild+"-vulkan")
	}
	return filepath.Join(dir, "runtime-"+llamaBuild)
}

// recommendBrain picks the model for this PC (graphics card first, then
// memory): see recommendFor.
func recommendBrain() (string, uint64) {
	hw := hardware(false)
	return recommendFor(hw).Model, hw.RAM
}

func mirrorURL(js JivSettings, file, def string) string {
	if m := strings.TrimRight(strings.TrimSpace(js.Mirror), "/"); strings.HasPrefix(m, "https://") {
		return m + "/" + file
	}
	return def
}

// brainHTTP has no overall timeout (multi-GB files); dial/headers time out.
var brainHTTP = &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 20 * time.Second}).DialContext,
	TLSHandshakeTimeout: 20 * time.Second, ResponseHeaderTimeout: 60 * time.Second}}

// fetchVerified downloads url to dst, resuming dst+".part" when possible,
// and checks size and sha256 before renaming it into place.
func fetchVerified(ctx context.Context, j *Job, label, u, dst, sum string, size int64, lo, hi int) error {
	if fileExists(dst) {
		if st, _ := os.Stat(dst); st.Size() == size {
			if err := verifySHA(dst, sum); err == nil {
				j.Logf("%s is already downloaded.", label)
				return nil
			}
		}
	}
	part := dst + ".part"
	h := sha256.New()
	var have int64
	if f, err := os.Open(part); err == nil {
		n, _ := io.Copy(h, f)
		f.Close()
		have = n
		if have > size {
			have = 0
			h.Reset()
		}
	}
	for attempt := 0; ; attempt++ {
		err := fetchRange(ctx, j, label, u, part, h, &have, size, lo, hi)
		if err == nil {
			break
		}
		if ctx.Err() != nil || attempt >= 4 {
			return err
		}
		j.Logf("%s: %v - retrying...", label, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(2+attempt*3) * time.Second):
		}
	}
	if have != size {
		return fmt.Errorf("%s: got %s, expected %s", label, humanBytes(have), humanBytes(size))
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, sum) {
		_ = os.Remove(part)
		return fmt.Errorf("%s failed the safety check (sha256 %s..., expected %s...). The file was deleted; please try again", label, got[:12], sum[:12])
	}
	j.Logf("%s verified (sha256 %s...).", label, sum[:16])
	return os.Rename(part, dst)
}

func fetchRange(ctx context.Context, j *Job, label, u, part string, h io.Writer, have *int64, size int64, lo, hi int) error {
	if *have >= size {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Rover/"+Version)
	if *have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", *have))
	}
	resp, err := brainHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("cannot connect (%v)", plainNetErr(err))
	}
	defer resp.Body.Close()
	flag := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	switch {
	case resp.StatusCode == http.StatusPartialContent && *have > 0:
	case resp.StatusCode == http.StatusOK:
		if *have > 0 { // server ignored Range: start over
			*have = 0
			if hh, ok := h.(interface{ Reset() }); ok {
				hh.Reset()
			}
		}
		flag = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		return nil
	default:
		return fmt.Errorf("download failed (HTTP %d)", resp.StatusCode)
	}
	f, err := os.OpenFile(part, flag, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 1<<20)
	last := time.Time{}
	start, startHave := time.Now(), *have
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if *have+int64(n) > size {
				return errors.New("the file is bigger than expected")
			}
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			h.Write(buf[:n])
			*have += int64(n)
			if time.Since(last) > 400*time.Millisecond {
				last = time.Now()
				pct := lo + int(float64(hi-lo)*float64(*have)/float64(size))
				speed := ""
				if el := time.Since(start).Seconds(); el > 1 {
					bps := float64(*have-startHave) / el
					speed = fmt.Sprintf(", %s/s", humanBytes(int64(bps)))
					if bps > 0 {
						speed += fmt.Sprintf(", about %s left", humanDur(time.Duration(float64(size-*have)/bps)*time.Second))
					}
				}
				j.SetProgress(pct, fmt.Sprintf("%s: %s of %s%s", label, humanBytes(*have), humanBytes(size), speed))
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return fmt.Errorf("download interrupted (%v)", rerr)
		}
	}
}

func humanDur(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()+0.5))
	}
	return fmt.Sprintf("%.1f h", d.Hours())
}

func verifySHA(p, sum string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), sum) {
		return errors.New("checksum mismatch")
	}
	return nil
}

// unzipRuntime extracts the llama.cpp zip (flattened) into dir.
func unzipRuntime(zipPath, dir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("the engine zip is damaged: %v", err)
	}
	defer zr.Close()
	tmp := dir + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	found := false
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		low := strings.ToLower(name)
		if !(strings.HasSuffix(low, ".exe") || strings.HasSuffix(low, ".dll") || strings.HasPrefix(low, "license") || !strings.Contains(low, ".")) {
			continue
		}
		if name == serverExeName() || name == "llama-server.exe" {
			found = true
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(filepath.Join(tmp, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(rc, 512<<20))
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	if !found {
		return errors.New("llama-server was not found in the engine zip")
	}
	_ = os.RemoveAll(dir)
	return os.Rename(tmp, dir)
}

// installBrainJob downloads the engine and the model, verifies both and
// starts the brain.
func installBrainJob(j *Job, id string, gpu bool) error {
	m, ok := brainModelByID(id)
	if !ok {
		return errors.New("unknown model")
	}
	dir, err := brainDir()
	if err != nil {
		return err
	}
	c := snapshotConfig()
	js := jivSettings(&c)
	rtKind := "cpu"
	if gpu {
		rtKind = "vulkan"
	}
	rt := brainRuntimes[rtKind]
	j.SetSteps("Downloading the AI engine (llama.cpp, "+humanBytes(rt.Size)+")", "Downloading the model ("+humanBytes(m.Size)+")", "Starting the offline brain")
	ctx := context.Background()
	brainSidecar.Stop()
	j.Step(0)
	zipPath := filepath.Join(dir, rt.File)
	if err := fetchVerified(ctx, j, "AI engine", mirrorURL(js, rt.File, llamaReleaseBase+rt.File), zipPath, rt.SHA256, rt.Size, 0, 4); err != nil {
		return err
	}
	if runtime.GOOS == "windows" || envOr("BRAIN_UNZIP") == "1" {
		if err := unzipRuntime(zipPath, runtimeDirFor(dir, rtKind)); err != nil {
			return err
		}
	}
	j.Step(1)
	if err := fetchVerified(ctx, j, "Model", mirrorURL(js, m.File, m.URL), filepath.Join(dir, m.File), m.SHA256, m.Size, 4, 98); err != nil {
		return err
	}
	_ = withConfig(func(c *Config) error {
		if c.Jiv == nil {
			c.Jiv = defaultJivSettings()
		}
		c.Jiv.Model, c.Jiv.ModelPath, c.Jiv.Runtime, c.Jiv.BrainOff, c.Jiv.GPU = m.ID, "", "", false, gpu
		return nil
	})
	j.Step(2)
	sctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if _, _, err := brainSidecar.Ensure(sctx); err != nil {
		return fmt.Errorf("downloaded, but the brain did not start: %v", err)
	}
	j.SetResult(map[string]any{"model": m.ID})
	j.Logf("The offline brain is ready. Jiv now uses %s on this PC.", m.Short)
	return nil
}

// importBrain uses files the user already has (no download): a .gguf model
// and a llama.cpp folder or zip.
func importBrain(modelPath, runtimePath string) (string, error) {
	modelPath, runtimePath = strings.Trim(strings.TrimSpace(modelPath), `"`), strings.Trim(strings.TrimSpace(runtimePath), `"`)
	f, err := os.Open(modelPath)
	if err != nil {
		return "", errors.New("cannot open the model file")
	}
	magic := make([]byte, 4)
	_, _ = io.ReadFull(f, magic)
	st, _ := f.Stat()
	f.Close()
	if string(magic) != "GGUF" {
		return "", errors.New("that file is not a GGUF model (it should end in .gguf)")
	}
	id := "custom"
	for _, m := range brainModels {
		if st.Size() == m.Size && verifySHA(modelPath, m.SHA256) == nil {
			id = m.ID
		}
	}
	dir, err := brainDir()
	if err != nil {
		return "", err
	}
	rt := ""
	if runtimePath != "" {
		info, err := os.Stat(runtimePath)
		switch {
		case err != nil:
			return "", errors.New("cannot open the llama.cpp file or folder")
		case info.IsDir():
			if !fileExists(filepath.Join(runtimePath, serverExeName())) {
				return "", errors.New("that folder has no " + serverExeName())
			}
			rt = runtimePath
		default:
			rt = filepath.Join(dir, "runtime-imported")
			if err := unzipRuntime(runtimePath, rt); err != nil {
				return "", err
			}
		}
	} else if !fileExists(filepath.Join(runtimeDir(dir, false), serverExeName())) {
		return "", errors.New("also choose the llama.cpp engine (the llama-…-bin-win-cpu-x64.zip file or its folder)")
	}
	brainSidecar.Stop()
	err = withConfig(func(c *Config) error {
		if c.Jiv == nil {
			c.Jiv = defaultJivSettings()
		}
		c.Jiv.Model, c.Jiv.ModelPath, c.Jiv.Runtime, c.Jiv.BrainOff = id, modelPath, rt, false
		return nil
	})
	return id, err
}

func removeBrain() error {
	brainSidecar.Stop()
	dir, err := brainDir()
	if err != nil {
		return err
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		n := e.Name()
		if strings.HasSuffix(n, ".gguf") || strings.HasSuffix(n, ".part") || strings.HasPrefix(n, "runtime-") || strings.HasPrefix(n, "llama-") {
			_ = os.RemoveAll(filepath.Join(dir, n))
		}
	}
	return withConfig(func(c *Config) error {
		if c.Jiv != nil {
			c.Jiv.Model, c.Jiv.ModelPath, c.Jiv.Runtime = "", "", ""
		}
		return nil
	})
}

// ---------- sidecar ----------

type sidecar struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	port     int
	key      string
	state    string // stopped | starting | ready | error
	err      string
	lastUse  time.Time
	model    string
	crashes  int
	started  time.Time
	exited   chan struct{}
	stopping bool
	gpu      bool   // this run offloads to the graphics card
	cpuOnly  bool   // the GPU run failed: stay on the processor until restart
	note     string // e.g. "the graphics card could not load the model; using the processor"
}

var brainSidecar = &sidecar{state: "stopped"}

// startHook lets tests replace the command (mock llama-server).
var brainCommand = func(exe string, args []string) *exec.Cmd { return exec.Command(exe, args...) }

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (s *sidecar) Status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{"state": s.state, "error": s.err, "model": s.model, "gpu": s.gpu, "note": s.note}
	if s.state == "ready" {
		out["since"] = s.started.UTC().Format(time.RFC3339)
	}
	return out
}

// Ensure starts the brain if needed and waits until it answers /health.
func (s *sidecar) Ensure(ctx context.Context) (base, key string, err error) {
	c := snapshotConfig()
	exe, model, m, ok := brainPaths(&c)
	if !ok {
		return "", "", errors.New("the offline brain is not installed")
	}
	s.mu.Lock()
	if s.state == "ready" && s.cmd != nil && s.model == m.ID {
		s.lastUse = time.Now()
		base, key = fmt.Sprintf("http://127.0.0.1:%d/v1", s.port), s.key
		s.mu.Unlock()
		return base, key, nil
	}
	if s.state != "starting" {
		if s.cmd != nil {
			s.stopLocked()
		}
		if err := s.startLocked(exe, model, m, jivSettings(&c)); err != nil {
			s.state, s.err = "error", err.Error()
			s.mu.Unlock()
			return "", "", err
		}
	}
	port, k, exited := s.port, s.key, s.exited
	s.mu.Unlock()
	hc := &http.Client{Timeout: 2 * time.Second}
	for {
		r, err := hc.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
		if err == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				s.mu.Lock()
				if s.port == port && s.state == "starting" {
					s.state, s.err, s.started, s.lastUse = "ready", "", time.Now(), time.Now()
				}
				s.mu.Unlock()
				return fmt.Sprintf("http://127.0.0.1:%d/v1", port), k, nil
			}
		}
		select {
		case <-ctx.Done():
			return "", "", errors.New("the offline brain is still loading; try again in a moment")
		case <-exited:
			s.mu.Lock()
			e := s.err
			if s.gpu && !s.cpuOnly {
				// The graphics card could not load it (old driver, too little
				// memory): start again on the processor.
				s.cpuOnly, s.note = true, "The graphics card could not run the model, so Jiv uses the processor. Update the graphics driver and press Restart to try the card again."
				if err := s.startLocked(exe, model, m, jivSettings(&c)); err == nil {
					port, k, exited = s.port, s.key, s.exited
					s.mu.Unlock()
					continue
				}
			}
			s.mu.Unlock()
			return "", "", fmt.Errorf("the offline brain stopped while loading: %s", firstNonEmpty(e, "unknown error"))
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (s *sidecar) startLocked(exe, model string, m BrainModel, js JivSettings) error {
	port, err := freePort()
	if err != nil {
		return err
	}
	threads := js.Threads
	if threads <= 0 {
		threads = max(1, min(8, runtime.NumCPU()-1))
	}
	s.key = randHex(16)
	args := []string{"-m", model, "--host", "127.0.0.1", "--port", strconv.Itoa(port), "--api-key", s.key, "-c", "8192", "-np", "1",
		"--jinja", "--no-webui", "-cram", "256", "-t", strconv.Itoa(threads)}
	args = append(args, m.Args...)
	args = append(args, brainGPUArgs(js.GPU && !s.cpuOnly)...)
	s.gpu = js.GPU && !s.cpuOnly
	cmd := brainCommand(exe, args)
	cmd.Dir = filepath.Dir(exe)
	hideConsole(cmd)
	if dir, err := brainDir(); err == nil {
		if lf, err := os.Create(filepath.Join(dir, "llama-server.log")); err == nil {
			cmd.Stdout, cmd.Stderr = lf, lf
			defer lf.Close() // the child keeps its own handle
		}
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start llama-server: %v", err)
	}
	s.cmd, s.port, s.state, s.err, s.model, s.stopping = cmd, port, "starting", "", m.ID, false
	s.exited = make(chan struct{})
	ch := s.exited
	go func() {
		werr := cmd.Wait()
		s.mu.Lock()
		if s.cmd == cmd {
			s.cmd = nil
			if !s.stopping {
				s.state, s.err = "error", "llama-server exited"
				if werr != nil {
					s.err = "llama-server exited: " + werr.Error()
				}
				if time.Since(s.started) < time.Minute {
					s.crashes++
				}
			} else {
				s.state = "stopped"
			}
		}
		s.mu.Unlock()
		close(ch)
	}()
	return nil
}

func (s *sidecar) stopLocked() {
	if s.cmd != nil && s.cmd.Process != nil {
		s.stopping = true
		_ = s.cmd.Process.Kill()
	}
	s.state = "stopped"
}

func (s *sidecar) Stop() {
	s.mu.Lock()
	s.cpuOnly, s.note = false, ""
	ch := s.exited
	running := s.cmd != nil
	s.stopLocked()
	s.mu.Unlock()
	if running && ch != nil {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
		}
	}
}

// idleCheck unloads the brain after IdleMin minutes without use.
func (s *sidecar) idleCheck(now time.Time, idle time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == "ready" && s.cmd != nil && now.Sub(s.lastUse) > idle {
		s.stopLocked()
		return true
	}
	return false
}

// brainAvailable: installed, switched on, and not crash-looping.
func brainAvailable(c *Config) bool {
	_, _, _, ok := brainPaths(c)
	if !ok || jivSettings(c).BrainOff {
		return false
	}
	brainSidecar.mu.Lock()
	defer brainSidecar.mu.Unlock()
	return brainSidecar.crashes < 3
}

// startBrainLoop auto-starts the brain (warm-up) and unloads it when idle.
func startBrainLoop(ctx context.Context) {
	go func() {
		c := snapshotConfig()
		if brainAvailable(&c) && !jivSettings(&c).NoAuto {
			sctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			_, _, _ = brainSidecar.Ensure(sctx)
			cancel()
		}
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				brainSidecar.Stop()
				return
			case now := <-t.C:
				c := snapshotConfig()
				idle := jivSettings(&c).IdleMin
				if idle <= 0 {
					idle = 20
				}
				brainSidecar.idleCheck(now, time.Duration(idle)*time.Minute)
			}
		}
	}()
}

func cpuCount() int { return runtime.NumCPU() }

// brainGPUArgs: with a GPU, leave -ngl unset so llama-server's --fit puts as
// many layers on the card as fit (keeping 512 MiB free) and the rest in RAM;
// flash attention plus an 8-bit KV cache halves the context's memory, which
// lets the Standard model work on 4 GB cards such as a GTX 1650. Without a
// GPU, -dev none keeps the Vulkan build on the processor.
func brainGPUArgs(gpu bool) []string {
	if gpu {
		return []string{"--fit", "on", "--fit-target", "512", "-fa", "on", "-ctk", "q8_0", "-ctv", "q8_0"}
	}
	return []string{"-dev", "none"}
}
