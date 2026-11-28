package prov

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

func init() { Register(supabaseProv{}) }

type supabaseProv struct{}

func (supabaseProv) Meta() Meta {
	return Meta{
		ID: "supabase", Name: "Supabase", Auth: "token",
		Docs: "https://supabase.com/docs/reference/api/introduction",
		Capabilities: map[Capability]Support{
			CapDatabase: SupportYes, CapAuth: SupportYes, CapStorage: SupportYes,
			CapHosting: SupportNo,
		},
		Fields: []Field{
			{Key: "token", Label: "Management API access token", Secret: true, Hint: "Account → Access Tokens (sbp_…)"},
			{Key: "project_ref", Label: "Project ref (optional)", Optional: true, Hint: "Needed to manage one project's database, Auth and Storage"},
			{Key: "service_role", Label: "Project service role key (optional)", Secret: true, Optional: true, Hint: "Project Settings → API. Never synced; stays on this PC."},
		},
		Scopes: []string{
			"A personal access token for the Management API (list/create projects).",
			"The project's service_role key only if you want Rover to list Auth users, tables or Storage buckets. It bypasses Row Level Security — treat it like a root password.",
		},
		Notes: "Hosting on Supabase is unsupported. Database/Auth/Storage calls go to the project's REST endpoints and need the service role key.",
	}
}

func (supabaseProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	if err := require(creds, "token"); err != nil {
		return nil, err
	}
	base := tr.BaseURL("supabase", "https://api.supabase.com")
	var projects []struct {
		ID      string `json:"id"`
		Ref     string `json:"ref"`
		Name    string `json:"name"`
		Region  string `json:"region"`
		Status  string `json:"status"`
		Created string `json:"created_at"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/v1/projects", bearer(creds["token"]), nil, &projects); err != nil {
		return nil, err
	}
	info := &Info{Health: "ok", Status: StatusConnected, Usage: map[string]any{"projects": len(projects)}, Who: fmt.Sprintf("%d Supabase project(s)", len(projects))}
	for _, p := range projects {
		info.Resources = append(info.Resources, Resource{ID: firstNonEmpty(p.Ref, p.ID), Name: p.Name, Kind: "project", Region: p.Region, Status: p.Status})
	}
	ref := strings.TrimSpace(creds["project_ref"])
	role := strings.TrimSpace(creds["service_role"])
	if ref != "" && role != "" {
		if err := supabaseProjectPing(ctx, tr, ref, role, info); err != nil {
			info.Health = "degraded"
			info.Usage["project_error"] = err.Error()
		}
	}
	return info, nil
}

func supabaseProjectPing(ctx context.Context, tr *Transport, ref, role string, info *Info) error {
	rest := tr.BaseURL("supabase-rest-"+ref, "https://"+ref+".supabase.co")
	hdr := map[string]string{"apikey": role, "Authorization": "Bearer " + role}
	// REST OpenAPI lists tables; a cheap call is HEAD or GET /rest/v1/ with Accept.
	var raw []byte
	if err := tr.Do(ctx, "GET", rest+"/rest/v1/", hdr, nil, &raw); err != nil {
		return fmt.Errorf("database REST: %w", err)
	}
	var buckets []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Public bool   `json:"public"`
	}
	if err := tr.DoJSON(ctx, "GET", rest+"/storage/v1/bucket", hdr, nil, &buckets); err == nil {
		for _, b := range buckets {
			info.Resources = append(info.Resources, Resource{ID: b.ID, Name: b.Name, Kind: "bucket"})
		}
		info.Usage["buckets"] = len(buckets)
	}
	var users struct {
		Users []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"users"`
	}
	if err := tr.DoJSON(ctx, "GET", rest+"/auth/v1/admin/users?page=1&per_page=5", hdr, nil, &users); err == nil {
		info.Usage["auth_users_sample"] = len(users.Users)
	}
	return nil
}

// SupabaseListTables uses PostgREST. Prefer this over raw SQL.
func SupabaseListTables(ctx context.Context, tr *Transport, projectRef, serviceRole string) ([]Resource, error) {
	rest := tr.BaseURL("supabase-rest-"+projectRef, "https://"+projectRef+".supabase.co")
	hdr := map[string]string{"apikey": serviceRole, "Authorization": "Bearer " + serviceRole, "Accept": "application/openapi+json"}
	var spec map[string]any
	if err := tr.DoJSON(ctx, "GET", rest+"/rest/v1/", hdr, nil, &spec); err != nil {
		return nil, err
	}
	var out []Resource
	if paths, ok := spec["paths"].(map[string]any); ok {
		for p := range paths {
			name := strings.TrimPrefix(p, "/")
			if name == "" || strings.Contains(name, "{") {
				continue
			}
			out = append(out, Resource{ID: name, Name: name, Kind: "table"})
		}
	}
	return out, nil
}

func supabaseProjectURL(ref string) string {
	return "https://" + url.PathEscape(ref) + ".supabase.co"
}
