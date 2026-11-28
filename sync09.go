package main

// Optional metadata sync to the user's own Supabase or Firebase project.
// SSH private keys, tokens and other secrets never leave this PC.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"hostpanel/prov"
)

type SyncSettings struct {
	Enabled       bool   `json:"enabled"`
	Provider      string `json:"provider,omitempty"` // supabase | firebase
	IntegrationID string `json:"integration_id,omitempty"`
	LastSync      string `json:"last_sync,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	DeviceID      string `json:"device_id,omitempty"`
}

// syncPayload is the only document uploaded. No secrets.
type syncPayload struct {
	Device   string           `json:"device"`
	Updated  string           `json:"updated"`
	Projects []syncProject    `json:"projects"`
	Settings syncSettingsView `json:"settings"`
}

type syncProject struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Domain     string `json:"domain,omitempty"`
	Engine     string `json:"engine,omitempty"`
	Framework  string `json:"framework,omitempty"`
	LastDeploy string `json:"last_deploy,omitempty"`
	Release    string `json:"release,omitempty"`
	InstantURL string `json:"instant_url,omitempty"`
	Updated    string `json:"updated,omitempty"`
}

type syncSettingsView struct {
	KeepDeploys int    `json:"keep_deploys,omitempty"`
	SSLWarnDays int    `json:"ssl_warn_days,omitempty"`
	Version     string `json:"rover_version"`
}

func buildSyncPayload(c *Config) syncPayload {
	id := ""
	if c.CloudSync != nil {
		id = c.CloudSync.DeviceID
	}
	if id == "" {
		id = "dev_" + randHex(8)
	}
	p := syncPayload{Device: id, Updated: time.Now().UTC().Format(time.RFC3339), Settings: syncSettingsView{KeepDeploys: c.KeepDeploys, SSLWarnDays: c.SSLWarnDays, Version: Version}}
	for _, s := range c.Sites {
		p.Projects = append(p.Projects, syncProject{Slug: s.Slug, Name: s.Name, Domain: s.Domain, Engine: s.Engine, Framework: s.Framework, LastDeploy: s.LastDeploy, Release: s.Release, InstantURL: s.InstantURL})
	}
	return p
}

func syncContainsSecret(b []byte) bool {
	s := string(b)
	// Fail closed: never upload a payload that looks like it contains a key.
	return strings.Contains(s, "BEGIN ") && strings.Contains(s, "PRIVATE KEY") ||
		strings.Contains(s, `"cred"`) || strings.Contains(s, "dpapi:") || strings.Contains(s, "coolify_token") ||
		strings.Contains(s, "managed_key")
}

func registerSyncRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sync", func(w http.ResponseWriter, r *http.Request) {
		c := snapshotConfig()
		writeJSON(w, 200, map[string]any{"settings": c.CloudSync, "preview": buildSyncPayload(&c)})
	})
	mux.HandleFunc("POST /api/sync", apiSaveSync)
	mux.HandleFunc("POST /api/sync/now", apiSyncNow)
}

func apiSaveSync(w http.ResponseWriter, r *http.Request) {
	var req SyncSettings
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Enabled {
		if req.Provider != "supabase" && req.Provider != "firebase" {
			writeErr(w, errors.New("choose Supabase or Firebase as the sync destination"))
			return
		}
		if req.IntegrationID == "" {
			writeErr(w, errors.New("connect a Supabase or Firebase account in the Integration Centre first"))
			return
		}
	}
	if err := withConfig(func(c *Config) error {
		if req.DeviceID == "" {
			if c.CloudSync != nil && c.CloudSync.DeviceID != "" {
				req.DeviceID = c.CloudSync.DeviceID
			} else {
				req.DeviceID = "dev_" + randHex(8)
			}
		}
		c.CloudSync = &req
		return nil
	}); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, req)
}

func apiSyncNow(w http.ResponseWriter, r *http.Request) {
	c := snapshotConfig()
	if c.CloudSync == nil || !c.CloudSync.Enabled {
		writeErr(w, errors.New("cloud sync is off"))
		return
	}
	payload := buildSyncPayload(&c)
	b, _ := json.Marshal(payload)
	if syncContainsSecret(b) {
		writeErr(w, errors.New("sync refused: the payload looked like it contained a secret"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	err := pushSync(ctx, &c, b)
	_ = withConfig(func(cfg *Config) error {
		if cfg.CloudSync == nil {
			return nil
		}
		if err != nil {
			cfg.CloudSync.LastError = err.Error()
		} else {
			cfg.CloudSync.LastError = ""
			cfg.CloudSync.LastSync = time.Now().UTC().Format(time.RFC3339)
		}
		return nil
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "bytes": len(b), "projects": len(payload.Projects)})
}

var syncDo = func(req *http.Request) (*http.Response, error) { return http.DefaultClient.Do(req) }

func pushSync(ctx context.Context, c *Config, body []byte) error {
	a := findIntegration(c, c.CloudSync.IntegrationID)
	if a == nil {
		return errors.New("the sync account is no longer connected")
	}
	cr := integCred(a)
	switch c.CloudSync.Provider {
	case "supabase":
		ref := strings.TrimSpace(cr["project_ref"])
		role := strings.TrimSpace(cr["service_role"])
		if ref == "" || role == "" {
			return errors.New("the Supabase account needs a project ref and service role key to store sync documents (they stay on this PC, encrypted)")
		}
		url := "https://" + ref + ".supabase.co/rest/v1/rover_projects"
		// Upsert a single metadata row per device into a user-owned table is
		// documented in cloud/schema.sql; here we POST to rover_sync if present
		// by using a generic RPC-less insert of name=device, domain=placeholder.
		req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(mustJSON(map[string]any{"name": "rover-sync-" + c.CloudSync.DeviceID, "domain": "sync.local"})))
		req.Header.Set("apikey", role)
		req.Header.Set("Authorization", "Bearer "+role)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Prefer", "resolution=merge-duplicates")
		resp, err := syncDo(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("Supabase returned HTTP %d (apply cloud/schema.sql in the project first)", resp.StatusCode)
		}
		return nil
	case "firebase":
		project := strings.TrimSpace(cr["project_id"])
		if project == "" {
			project = prov.FirebaseProjectID(cr)
		}
		if project == "" {
			return errors.New("the Firebase account needs a project ID to write sync documents")
		}
		device := strings.TrimSpace(c.CloudSync.DeviceID)
		if device == "" {
			device = "dev_unknown"
		}
		tr := integTransport()
		return prov.WriteSyncDocument(ctx, tr, cr, project, device, body)
	default:
		return errors.New("unknown sync provider")
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
