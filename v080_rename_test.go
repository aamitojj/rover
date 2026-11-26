package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetRenames(t *testing.T) {
	t.Helper()
	reset := func() {
		renames.Lock()
		renames.moved, renames.notes = nil, nil
		renames.keptDocs, renames.keptLocal, renames.keptCfg = "", "", ""
		renames.Unlock()
		welcomeRename = false
	}
	reset()
	t.Cleanup(reset)
}

func TestEnvPrecedenceRoverFirst(t *testing.T) {
	t.Setenv("ROVER_X", "")
	t.Setenv("DILREET_X", "")
	t.Setenv("HOSTPANEL_X", "hp")
	if envOr("X") != "hp" {
		t.Fatal("HOSTPANEL_ fallback")
	}
	t.Setenv("DILREET_X", "dl")
	if envOr("X") != "dl" {
		t.Fatal("DILREET_ before HOSTPANEL_")
	}
	t.Setenv("ROVER_X", "rv")
	if envOr("X") != "rv" {
		t.Fatal("ROVER_ must win")
	}
}

// legacyHome builds a Windows-like profile used by Dilreet 0.7.
func legacyHome(t *testing.T) (home, roaming, local, docs string) {
	home = t.TempDir()
	roaming, local, docs = filepath.Join(home, "AppData", "Roaming"), filepath.Join(home, "AppData", "Local"), filepath.Join(home, "Documents")
	for _, k := range []string{"CONFIG_DIR", "PROJECTS_DIR", "BRAIN_DIR"} {
		t.Setenv("ROVER_"+k, "")
		t.Setenv("DILREET_"+k, "")
		t.Setenv("HOSTPANEL_"+k, "")
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", roaming)
	t.Setenv("XDG_CACHE_HOME", local)
	oldCfg := filepath.Join(roaming, "Dilreet")
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(oldCfg, "manifests"), 0o700))
	must(os.MkdirAll(filepath.Join(docs, "Dilreet", "Projects", "Bakery", "current"), 0o700))
	must(os.MkdirAll(filepath.Join(local, "Dilreet", "Brain"), 0o700))
	must(os.WriteFile(filepath.Join(docs, "Dilreet", "Projects", "Bakery", "current", "index.html"), []byte("<h1>hi</h1>"), 0o600))
	must(os.WriteFile(filepath.Join(local, "Dilreet", "Brain", "model.gguf"), []byte("GGUF"), 0o600))
	must(os.WriteFile(filepath.Join(oldCfg, "manifests", "bakery.json"), []byte("{}"), 0o600))
	cfgJSON := `{"config_version":7,"server_ip":"203.0.113.9","sites":[{"slug":"bakery","name":"Bakery","last_source":` +
		jsonStr(filepath.Join(docs, "Dilreet", "Projects", "Bakery", "current")) + `}],"jiv":{"model":"qwen3-1.7b","model_path":` +
		jsonStr(filepath.Join(local, "Dilreet", "Brain", "model.gguf")) + `}}`
	must(os.WriteFile(filepath.Join(oldCfg, "config.json"), []byte(cfgJSON), 0o600))
	// Rover already wrote its log before the move (platformInit runs first).
	must(os.MkdirAll(filepath.Join(local, "Rover"), 0o700))
	must(os.WriteFile(filepath.Join(local, "Rover", "rover.log"), []byte("log"), 0o600))
	return
}

func jsonStr(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

func TestRenameMovesDilreetFolders(t *testing.T) {
	resetRenames(t)
	_, roaming, local, docs := legacyHome(t)
	migrateLegacyDirs(legacyDirs{Roaming: roaming, Local: local, Docs: docs})
	if err := loadConfig(); err != nil {
		t.Fatal(err)
	}
	c := snapshotConfig()
	if c.ServerIP != "203.0.113.9" || len(c.Sites) != 1 {
		t.Fatalf("settings not carried over: %+v", c)
	}
	newSrc := filepath.Join(docs, "Rover", "Projects", "Bakery", "current")
	if c.Sites[0].LastSource != newSrc {
		t.Fatalf("saved path not rewritten: %q", c.Sites[0].LastSource)
	}
	if b, err := os.ReadFile(filepath.Join(newSrc, "index.html")); err != nil || string(b) != "<h1>hi</h1>" {
		t.Fatal("projects not moved", err)
	}
	if got := jivSettings(&c).ModelPath; got != filepath.Join(local, "Rover", "Brain", "model.gguf") || !fileExists(got) {
		t.Fatalf("brain path %q", got)
	}
	if !fileExists(filepath.Join(roaming, "Rover", "manifests", "bakery.json")) || !fileExists(filepath.Join(local, "Rover", "rover.log")) {
		t.Fatal("sub-folders or the new log were lost")
	}
	for _, d := range []string{filepath.Join(roaming, "Dilreet"), filepath.Join(docs, "Dilreet"), filepath.Join(local, "Dilreet")} {
		if !fileExists(filepath.Join(d, "MOVED-TO-ROVER.txt")) {
			t.Errorf("no note left in %s", d)
		}
	}
	if p, _ := configPath(); p != filepath.Join(roaming, "Rover", "config.json") {
		t.Fatalf("config path %s", p)
	}
	if projectsRoot(&c) != filepath.Join(docs, "Rover", "Projects") || docsAppDir() != filepath.Join(docs, "Rover") {
		t.Fatalf("projects root %s", projectsRoot(&c))
	}
	if !welcomeRename || c.ConfigVersion != currentConfigVersion {
		t.Fatal("rename notice / config version")
	}
	// second start: nothing to do
	resetRenames(t)
	migrateLegacyDirs(legacyDirs{Roaming: roaming, Local: local, Docs: docs})
	if len(renames.moved) != 0 || legacyKept("docs") != "" {
		t.Fatal("migration must run once")
	}
}

func TestRenameFallsBackWhenFoldersCannotMove(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores folder permissions")
	}
	resetRenames(t)
	_, roaming, local, docs := legacyHome(t)
	// Documents is read-only: Documents\Dilreet cannot be renamed.
	if err := os.Chmod(docs, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(docs, 0o700) })
	migrateLegacyDirs(legacyDirs{Roaming: roaming, Local: local, Docs: docs})
	if err := loadConfig(); err != nil {
		t.Fatal(err)
	}
	c := snapshotConfig()
	old := filepath.Join(docs, "Dilreet", "Projects")
	if c.ProjectsDir != old || projectsRoot(&c) != old || c.Sites[0].LastSource != filepath.Join(old, "Bakery", "current") {
		t.Fatalf("must keep using the old projects folder: %q %q", c.ProjectsDir, c.Sites[0].LastSource)
	}
	if !fileExists(filepath.Join(old, "Bakery", "current", "index.html")) {
		t.Fatal("projects lost")
	}
	if docsAppDir() != filepath.Join(docs, "Dilreet") {
		t.Fatal("backups and other Documents folders must stay with the projects")
	}
}

func TestUpdaterAcceptsDilreetAssetNames(t *testing.T) {
	r := &ghRelease{Tag: "v0.8.0", Assets: []ghAsset{{Name: "dilreet.exe"}, {Name: "Dilreet-Setup-0.8.0.exe"}}}
	if a, k := pickAsset(r, "windows", true); a == nil || a.Name != "Dilreet-Setup-0.8.0.exe" || k != "installer" {
		t.Fatalf("installer under the old name: %v %s", a, k)
	}
	if a, k := pickAsset(r, "windows", false); a == nil || a.Name != "dilreet.exe" || k != "exe" {
		t.Fatalf("portable under the old name: %v %s", a, k)
	}
	r.Assets = append(r.Assets, ghAsset{Name: "Rover-Offline-Setup-0.8.0.exe"}, ghAsset{Name: "Rover-Setup-0.8.0.exe"}, ghAsset{Name: "rover.exe"})
	if a, _ := pickAsset(r, "windows", true); a == nil || a.Name != "Rover-Setup-0.8.0.exe" {
		t.Fatalf("Rover names first, never the offline installer: %v", a)
	}
	if a, _ := pickAsset(r, "windows", false); a == nil || a.Name != "rover.exe" {
		t.Fatalf("rover.exe first: %v", a)
	}
}
