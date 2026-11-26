package main

// Rover 0.8: offline packs. Everything Jiv needs can come from a file or
// a USB stick instead of the internet:
//
//   - "Rover Offline (full)" installer: puts the Light model and both
//     llama.cpp engines straight into the brain folder; on first start the
//     app checks them against its pinned SHA-256 values and turns them on.
//   - .dlpack files (one file, or .dlpack.001/.002 parts under 2 GB each):
//     Settings > Jiv > Import offline pack, or dropped in <app>\offline.
//   - Export: write the installed brain to a USB stick for another PC.
//
// Security: a file from a pack is used only when its SHA-256 equals a value
// built into Rover (brainModels / brainRuntimes), so a tampered pack can
// never run an unknown program.

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"hostpanel/dlpack"
)

// pinned looks a pack entry up in the built-in catalogue by checksum.
func pinned(sum string) (kind, id, file string, size int64, ok bool) {
	sum = strings.ToLower(sum)
	for _, m := range brainModels {
		if m.SHA256 == sum {
			return "model", m.ID, m.File, m.Size, true
		}
	}
	for k, r := range brainRuntimes {
		if r.SHA256 == sum {
			return "runtime", k, r.File, r.Size, true
		}
	}
	return "", "", "", 0, false
}

// offlineDir is <folder of rover.exe>\offline (the full installer and
// IT departments put packs there).
var offlineDir = func() string {
	if d := envOr("OFFLINE_DIR"); d != "" {
		return d
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "offline")
}

var packMu sync.Mutex // one import/export at a time

func installedRuntimes(dir string) []string {
	var out []string
	for _, k := range []string{"cpu", "vulkan"} {
		if fileExists(filepath.Join(runtimeDirFor(dir, k), serverExeName())) || fileExists(filepath.Join(dir, brainRuntimes[k].File)) {
			out = append(out, k)
		}
	}
	return out
}

func packStatus() map[string]any {
	dir, _ := brainDir()
	var found []map[string]any
	if od := offlineDir(); od != "" {
		for _, p := range dlpack.Find(od) {
			if pk, err := dlpack.Open(p); err == nil {
				found = append(found, map[string]any{"path": p, "title": pk.Manifest.Title, "size": pk.Size, "parts": len(pk.Parts)})
				pk.Close()
			}
		}
	}
	var models []string
	for _, m := range brainModels {
		if fileExists(filepath.Join(dir, m.File)) {
			models = append(models, m.ID)
		}
	}
	return map[string]any{"dir": dir, "offline_dir": offlineDir(), "found": found, "models": models, "runtimes": installedRuntimes(dir),
		"kb_articles": len(jivKB().docs), "guides": len(loadGuides())}
}

// importPackJob installs a pack (any part of a split pack, or a folder that
// holds one).
func importPackJob(j *Job, path string) error {
	packMu.Lock()
	defer packMu.Unlock()
	path = strings.Trim(strings.TrimSpace(path), `"`)
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		ps := dlpack.Find(path)
		if len(ps) == 0 {
			return errors.New("no Rover offline pack (.dlpack) in that folder")
		}
		path = ps[0]
	}
	pk, err := dlpack.Open(path)
	if err != nil {
		return err
	}
	defer pk.Close()
	dir, err := brainDir()
	if err != nil {
		return err
	}
	var total int64
	for _, f := range pk.Manifest.Files {
		total += f.Size
	}
	j.SetSteps("Checking the pack", "Copying the files ("+humanBytes(total)+")", "Turning on the offline brain")
	j.Step(0)
	j.Logf("Pack: %s (%d part(s), %s)", firstNonEmpty(pk.Manifest.Title, filepath.Base(path)), len(pk.Parts), humanBytes(pk.Size))
	j.Step(1)
	brainSidecar.Stop()
	var done int64
	model, gotVulkan, gotCPU := "", false, false
	for _, f := range pk.Manifest.Files {
		kind, id, file, size, ok := pinned(f.SHA256)
		if !ok || size != f.Size {
			j.Logf("Skipped %s: not a file this version of Rover knows (it only installs files with built-in checksums).", f.Name)
			continue
		}
		dst := filepath.Join(dir, file)
		if fileExists(dst) && verifySHA(dst, f.SHA256) == nil {
			j.Logf("%s is already installed.", file)
		} else {
			j.Logf("Copying %s (%s)…", file, humanBytes(size))
			base := done
			sum, n, err := pk.Extract(f.Name, dst, func(n int64) {
				if total > 0 {
					j.SetProgress(int((base+n)*100/total), "Copying "+file)
				}
			})
			if err != nil {
				return fmt.Errorf("copying %s failed: %v", file, err)
			}
			if !strings.EqualFold(sum, f.SHA256) || n != size {
				os.Remove(dst)
				return fmt.Errorf("%s is damaged (checksum mismatch). Copy the pack again", file)
			}
		}
		done += size
		switch kind {
		case "model":
			if model == "" || id == "qwen3-4b" {
				model = id
			}
		case "runtime":
			if runtime.GOOS == "windows" || envOr("BRAIN_UNZIP") == "1" {
				if err := unzipRuntime(dst, runtimeDirFor(dir, id)); err != nil {
					return err
				}
			}
			gotVulkan = gotVulkan || id == "vulkan"
			gotCPU = gotCPU || id == "cpu"
		}
	}
	if model == "" && !gotCPU && !gotVulkan {
		return errors.New("this pack has nothing Rover can use")
	}
	j.Step(2)
	return activateBrainFiles(j, model)
}

// activateBrainFiles turns the brain on after files arrived (pack import,
// full installer). GPU use follows the hardware advice.
func activateBrainFiles(j *Job, model string) error {
	dir, _ := brainDir()
	c := snapshotConfig()
	js := jivSettings(&c)
	if model == "" {
		model = js.Model
	}
	if model == "" {
		j.Logf("Engine installed. Add a model (Settings > Jiv) to use the offline brain.")
		return nil
	}
	adv := recommendFor(hardware(false))
	gpu := adv.GPU && fileExists(filepath.Join(runtimeDirFor(dir, "vulkan"), serverExeName()))
	if runtime.GOOS != "windows" && envOr("BRAIN_UNZIP") != "1" {
		gpu = false
	}
	_ = withConfig(func(c *Config) error {
		if c.Jiv == nil {
			c.Jiv = defaultJivSettings()
		}
		c.Jiv.Model, c.Jiv.ModelPath, c.Jiv.Runtime, c.Jiv.BrainOff, c.Jiv.GPU = model, "", "", false, gpu
		return nil
	})
	m, _ := brainModelByID(model)
	where := "the processor"
	if gpu {
		where = "the graphics card (" + adv.Card + ")"
	}
	j.Logf("The offline brain is ready: %s, running on %s. Nothing needs the internet.", m.Short, where)
	j.SetResult(map[string]any{"model": model, "gpu": gpu})
	return nil
}

// adoptInstalledBrain handles the full installer: model and engines are
// already in the brain folder. Runs once at start when no brain is set.
func adoptInstalledBrain() {
	c := snapshotConfig()
	if jivSettings(&c).Model != "" {
		return
	}
	dir, err := brainDir()
	if err != nil {
		return
	}
	model := ""
	for _, m := range brainModels {
		p := filepath.Join(dir, m.File)
		if fileExists(p) && model == "" {
			if st, _ := os.Stat(p); st.Size() == m.Size {
				model = m.ID
			}
		}
	}
	if model == "" {
		// a pack dropped in <app>\offline
		if od := offlineDir(); od != "" {
			if ps := dlpack.Find(od); len(ps) > 0 {
				newJob("pack-import", func(j *Job) error {
					err := importPackJob(j, ps[0])
					if err == nil {
						if pk, e := dlpack.Open(ps[0]); e == nil {
							parts := pk.Parts
							pk.Close()
							for _, p := range parts {
								_ = os.Remove(p) // free the space; ignore if read-only
							}
						}
					}
					return err
				})
			}
		}
		return
	}
	newJob("brain-adopt", func(j *Job) error {
		j.SetSteps("Checking the built-in offline brain", "Turning it on")
		j.Step(0)
		m, _ := brainModelByID(model)
		if err := verifySHA(filepath.Join(dir, m.File), m.SHA256); err != nil {
			return fmt.Errorf("the built-in model file is damaged: %v. Reinstall Rover Offline or download it in Settings > Jiv", err)
		}
		for k, rt := range brainRuntimes {
			zp := filepath.Join(dir, rt.File)
			if !fileExists(zp) || fileExists(filepath.Join(runtimeDirFor(dir, k), serverExeName())) {
				continue
			}
			if err := verifySHA(zp, rt.SHA256); err != nil {
				j.Logf("Skipped the %s engine: %v", k, err)
				continue
			}
			if runtime.GOOS == "windows" || envOr("BRAIN_UNZIP") == "1" {
				if err := unzipRuntime(zp, runtimeDirFor(dir, k)); err != nil {
					return err
				}
			}
		}
		j.Step(1)
		return activateBrainFiles(j, model)
	})
	log.Printf("offline brain: found built-in model %s, turning it on", model)
}

// exportPackJob writes the installed brain as a pack (for a USB stick).
func exportPackJob(j *Job, outDir string) error {
	packMu.Lock()
	defer packMu.Unlock()
	outDir = strings.Trim(strings.TrimSpace(outDir), `"`)
	if st, err := os.Stat(outDir); err != nil || !st.IsDir() {
		return errors.New("choose a folder (for example your USB stick)")
	}
	dir, err := brainDir()
	if err != nil {
		return err
	}
	var srcs []dlpack.Source
	title := "Rover offline brain"
	for _, m := range brainModels {
		if p := filepath.Join(dir, m.File); fileExists(p) {
			srcs = append(srcs, dlpack.Source{Path: p, File: dlpack.File{Name: m.File, Kind: "model", ID: m.ID, SHA256: m.SHA256, License: m.License}})
			title += " - " + m.Short
		}
	}
	for _, k := range []string{"cpu", "vulkan"} {
		rt := brainRuntimes[k]
		if p := filepath.Join(dir, rt.File); fileExists(p) {
			srcs = append(srcs, dlpack.Source{Path: p, File: dlpack.File{Name: rt.File, Kind: "runtime", ID: k, SHA256: rt.SHA256, License: rt.License}})
		}
	}
	if len(srcs) == 0 {
		return errors.New("nothing to export: the offline brain is not installed")
	}
	j.SetSteps("Writing the pack")
	j.Step(0)
	base := filepath.Join(outDir, "Rover-Offline-"+llamaBuild+".dlpack")
	files, err := dlpack.Write(base, title, llamaBuild, srcs, dlpack.DefaultPart, func(d, t int64) {
		if t > 0 {
			j.SetProgress(int(d*100/t), "Writing")
		}
	})
	if err != nil {
		return err
	}
	for _, f := range files {
		if st, err := os.Stat(f); err == nil {
			j.Logf("Wrote %s (%s)", filepath.Base(f), humanBytes(st.Size()))
		}
	}
	j.SetResult(map[string]any{"files": files})
	return nil
}

func registerPackRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/jiv/pack", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, packStatus())
	})
	mux.HandleFunc("POST /api/jiv/pack/import", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Path string }
		if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Path) == "" {
			writeErr(w, errors.New("choose the .dlpack file (or the folder or USB stick that has it)"))
			return
		}
		j := newJob("pack-import", func(j *Job) error { return importPackJob(j, req.Path) })
		writeJSON(w, 200, map[string]any{"job": j.ID})
	})
	mux.HandleFunc("POST /api/jiv/pack/export", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Dir string }
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		j := newJob("pack-export", func(j *Job) error { return exportPackJob(j, req.Dir) })
		writeJSON(w, 200, map[string]any{"job": j.ID})
	})
	mux.HandleFunc("GET /api/jiv/hardware", func(w http.ResponseWriter, r *http.Request) {
		hw := hardware(r.URL.Query().Get("refresh") == "1")
		writeJSON(w, 200, map[string]any{"hardware": hw, "advice": recommendFor(hw), "status": brainSidecar.Status()})
	})
}
