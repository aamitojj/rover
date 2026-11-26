package main

// Rover 0.8: the app was called Dilreet (0.4-0.7) and HostPanel before that.
// On first start Rover moves the old folders to their new names:
//
//	%APPDATA%\Dilreet        -> %APPDATA%\Rover        (settings, keys, history)
//	%LOCALAPPDATA%\Dilreet   -> %LOCALAPPDATA%\Rover   (offline brain, window data, log)
//	Documents\Dilreet        -> Documents\Rover        (projects and deploy history)
//
// A move is a rename (instant, no extra disk space). When Windows refuses
// (a file is open, the folder is on another drive), settings are copied
// instead and the big folders simply stay where they are: Rover keeps using
// them. Paths saved in the settings are rewritten to the new folders. The
// server side (/data/hostpanel, tunnel service names) is unchanged.

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	appName    = "Rover"
	legacyName = "Dilreet"
)

var renames struct {
	sync.Mutex
	moved     [][2]string // old -> new folder, for rewriting saved paths
	keptDocs  string      // Documents\Dilreet kept in place (move failed)
	keptLocal string      // %LOCALAPPDATA%\Dilreet kept in place
	keptCfg   string      // %APPDATA%\Dilreet kept in place (move and copy failed)
	notes     []string
}

// envOr reads ROVER_<name>, then the older DILREET_<name> and HOSTPANEL_<name>.
func envOr(name string) string {
	for _, p := range []string{"ROVER_", "DILREET_", "HOSTPANEL_"} {
		if v := os.Getenv(p + name); v != "" {
			return v
		}
	}
	return ""
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// moveDir renames old to new. When new already exists (for example the log
// file was just created there), the entries of old are moved into it one by
// one; entries that exist on both sides are left in old.
func moveDir(old, nw string) error {
	if !exists(old) {
		return fs.ErrNotExist
	}
	if err := os.MkdirAll(filepath.Dir(nw), 0o700); err != nil {
		return err
	}
	if !exists(nw) {
		if err := os.Rename(old, nw); err != nil {
			return err
		}
	} else {
		ents, err := os.ReadDir(old)
		if err != nil {
			return err
		}
		var left []string
		for _, e := range ents {
			src, dst := filepath.Join(old, e.Name()), filepath.Join(nw, e.Name())
			if exists(dst) {
				left = append(left, e.Name())
				continue
			}
			if err := os.Rename(src, dst); err != nil {
				return err
			}
		}
		if len(left) > 0 {
			log.Printf("rename: kept %v in %s (already in %s)", left, old, nw)
		}
	}
	// A note for anyone who opens the old place, and for older copies.
	if os.MkdirAll(old, 0o700) == nil {
		_ = os.WriteFile(filepath.Join(old, "MOVED-TO-ROVER.txt"), []byte("Dilreet is now called Rover.\r\nEverything that was here moved to:\r\n"+nw+"\r\n"), 0o600)
	}
	return nil
}

// migrateLegacyDirs runs once at start, before the settings are read.
// dirs can be overridden for tests.
type legacyDirs struct{ Roaming, Local, Docs string }

func defaultLegacyDirs() legacyDirs {
	var d legacyDirs
	if b, err := os.UserConfigDir(); err == nil && envOr("CONFIG_DIR") == "" {
		d.Roaming = b
	}
	if b, err := os.UserCacheDir(); err == nil && envOr("BRAIN_DIR") == "" {
		d.Local = b
	}
	if envOr("PROJECTS_DIR") == "" {
		d.Docs = documentsDir()
	}
	return d
}

func migrateLegacyDirs(d legacyDirs) {
	renames.Lock()
	defer renames.Unlock()
	note := func(s string) { renames.notes = append(renames.notes, s); log.Print(s) }
	if d.Roaming != "" {
		old, nw := filepath.Join(d.Roaming, legacyName), filepath.Join(d.Roaming, appName)
		if exists(filepath.Join(old, "config.json")) && !exists(filepath.Join(nw, "config.json")) {
			if err := moveDir(old, nw); err == nil && exists(filepath.Join(nw, "config.json")) {
				renames.moved = append(renames.moved, [2]string{old, nw})
				note("Moved settings from " + old + " to " + nw)
			} else if err := copyTree(nw, old); err == nil {
				renames.moved = append(renames.moved, [2]string{old, nw})
				note("Copied settings from " + old + " to " + nw + " (the old folder is kept)")
			} else {
				renames.keptCfg = old
				note("Could not move settings (" + err.Error() + "); still using " + old)
			}
		}
	}
	if d.Local != "" {
		old, nw := filepath.Join(d.Local, legacyName), filepath.Join(d.Local, appName)
		if exists(old) && !exists(filepath.Join(old, "MOVED-TO-ROVER.txt")) && !exists(filepath.Join(nw, "Brain")) {
			err := moveDir(old, nw)
			if exists(filepath.Join(old, "Brain")) {
				renames.keptLocal = old
				note("Could not move " + old + " (" + errText(err) + "); the offline brain stays there")
			} else if err == nil {
				renames.moved = append(renames.moved, [2]string{old, nw})
				note("Moved app data from " + old + " to " + nw)
			}
		}
	}
	if d.Docs != "" {
		old, nw := filepath.Join(d.Docs, legacyName), filepath.Join(d.Docs, appName)
		if exists(filepath.Join(old, "Projects")) && !exists(filepath.Join(nw, "Projects")) {
			err := moveDir(old, nw)
			if err == nil && exists(filepath.Join(nw, "Projects")) {
				renames.moved = append(renames.moved, [2]string{old, nw})
				note("Moved projects from " + old + " to " + nw)
			} else {
				renames.keptDocs = old
				note("Could not move " + old + " (" + errText(err) + "); projects stay there")
			}
		}
	}
}

// rewriteMovedPaths updates folder paths saved in config.json.
func rewriteMovedPaths(b []byte) ([]byte, bool) {
	renames.Lock()
	defer renames.Unlock()
	changed := false
	for _, m := range renames.moved {
		oq, _ := json.Marshal(m[0])
		nq, _ := json.Marshal(m[1])
		o, n := bytes.Trim(oq, `"`), bytes.Trim(nq, `"`)
		if bytes.Contains(b, o) {
			b = bytes.ReplaceAll(b, o, n)
			changed = true
		}
	}
	return b, changed
}

// legacyKept returns the old folder Rover keeps using for kind
// ("docs", "local", "config"), or "".
func legacyKept(kind string) string {
	renames.Lock()
	defer renames.Unlock()
	switch kind {
	case "docs":
		return renames.keptDocs
	case "local":
		return renames.keptLocal
	case "config":
		return renames.keptCfg
	}
	return ""
}

// localDataDir is %LOCALAPPDATA%\Rover (or the old Dilreet one when it could
// not be moved).
func localDataDir() (string, error) {
	if k := legacyKept("local"); k != "" {
		return k, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName), nil
}

// isLegacyAsset: release files named for Dilreet are accepted by the updater.
func isLegacyName(n string) bool { return strings.Contains(strings.ToLower(n), "dilreet") }

func errText(err error) string {
	if err == nil {
		return "some files are in use"
	}
	return err.Error()
}

// docsAppDir is Documents\Rover (or Documents\Dilreet when it could not be moved).
func docsAppDir() string {
	if k := legacyKept("docs"); k != "" {
		return k
	}
	return filepath.Join(documentsDir(), appName)
}
