package prov

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	Register(cloudflareProv{})
	Register(githubProv{})
	Register(gitlabProv{})
}

type cloudflareProv struct{}

func (cloudflareProv) Meta() Meta {
	return Meta{
		ID: "cloudflare", Name: "Cloudflare", Auth: "token",
		Docs: "https://developers.cloudflare.com/api/",
		Capabilities: map[Capability]Support{
			CapDNS: SupportYes, CapTunnels: SupportYes, CapAnalytics: SupportYes, CapHosting: SupportNo,
		},
		Fields: []Field{
			{Key: "token", Label: "API token", Secret: true, Hint: "A scoped token, not the Global API key"},
			{Key: "account_id", Label: "Account ID (for tunnels and account analytics)", Optional: true},
		},
		Scopes: []string{
			"Zone → Zone:Read, Zone → DNS:Edit (for records Rover writes), Account → Cloudflare Tunnel:Edit (only if you use tunnels), Account → Account Analytics:Read (for GraphQL analytics).",
		},
		Notes: "Uses the official REST API for zones/tunnels and the GraphQL analytics API. The Global API key is not required and should not be pasted.",
	}
}

func (cloudflareProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	if err := require(creds, "token"); err != nil {
		return nil, err
	}
	base := tr.BaseURL("cloudflare", "https://api.cloudflare.com")
	hdr := bearer(creds["token"])
	var tok struct {
		Success bool `json:"success"`
		Result  struct {
			ID       string `json:"id"`
			Status   string `json:"status"`
			Policies []struct {
				Effect string `json:"effect"`
			} `json:"policies"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/client/v4/user/tokens/verify", hdr, nil, &tok); err != nil {
		return nil, err
	}
	if !tok.Success && tok.Result.Status == "" {
		msg := "token check failed"
		if len(tok.Errors) > 0 {
			msg = tok.Errors[0].Message
		}
		return nil, fmt.Errorf("%s", msg)
	}
	info := &Info{Who: "Cloudflare token " + tok.Result.Status, Health: "ok", Status: StatusConnected}
	var zones struct {
		Success bool `json:"success"`
		Result  []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
			Plan   struct {
				Name string `json:"name"`
			} `json:"plan"`
		} `json:"result"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/client/v4/zones?per_page=50", hdr, nil, &zones); err == nil {
		info.Usage = map[string]any{"zones": len(zones.Result)}
		for _, z := range zones.Result {
			if info.Plan == "" {
				info.Plan = z.Plan.Name
			}
			info.Resources = append(info.Resources, Resource{ID: z.ID, Name: z.Name, Kind: "zone", Status: z.Status, Extra: z.Plan.Name})
		}
	}
	acct := strings.TrimSpace(creds["account_id"])
	if acct != "" {
		var tun struct {
			Success bool `json:"success"`
			Result  []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"result"`
		}
		if err := tr.DoJSON(ctx, "GET", base+"/client/v4/accounts/"+acct+"/cfd_tunnel?is_deleted=false", hdr, nil, &tun); err == nil {
			for _, t := range tun.Result {
				info.Resources = append(info.Resources, Resource{ID: t.ID, Name: t.Name, Kind: "tunnel"})
			}
			if info.Usage == nil {
				info.Usage = map[string]any{}
			}
			info.Usage["tunnels"] = len(tun.Result)
		}
	}
	return info, nil
}

// CloudflareAnalytics runs a GraphQL httpRequests1dGroups query for a zone.
func CloudflareAnalytics(ctx context.Context, tr *Transport, creds map[string]string, zoneID, since, until string) (map[string]any, error) {
	if err := require(map[string]string{"token": creds["token"]}, "token"); err != nil {
		return nil, err
	}
	base := tr.BaseURL("cloudflare", "https://api.cloudflare.com")
	q := fmt.Sprintf(`query { viewer { zones(filter: {zoneTag: %q}) { httpRequests1dGroups(limit: 30, filter: {date_geq: %q, date_leq: %q}) { dimensions { date } uniq { uniques } sum { requests bytes threats } } } } }`, zoneID, since, until)
	var out map[string]any
	if err := tr.DoJSON(ctx, "POST", base+"/client/v4/graphql", bearer(creds["token"]), map[string]any{"query": q}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

type githubProv struct{}

func (githubProv) Meta() Meta {
	return Meta{
		ID: "github", Name: "GitHub", Auth: "token", Optional: true,
		Docs:         "https://docs.github.com/en/rest",
		Capabilities: map[Capability]Support{CapImport: SupportYes, CapHosting: SupportNo},
		Fields:       []Field{{Key: "token", Label: "Personal access token", Secret: true, Hint: "repo (classic) or Contents: Read on the repositories you import"}},
		Scopes:       []string{"Fine-grained: Contents (read) on the repositories you want to import. Classic: repo for private repositories, public_repo for public only."},
		Notes:        "Optional import only. Rover never depends on GitHub at runtime to host a site.",
	}
}

func (githubProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	if err := require(creds, "token"); err != nil {
		return nil, err
	}
	base := tr.BaseURL("github", "https://api.github.com")
	hdr := bearer(creds["token"])
	hdr["Accept"] = "application/vnd.github+json"
	var user struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/user", hdr, nil, &user); err != nil {
		return nil, err
	}
	info := &Info{Who: firstNonEmpty(user.Name, user.Login), Health: "ok", Status: StatusConnected}
	var repos []struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
		Private  bool   `json:"private"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/user/repos?per_page=20&sort=updated", hdr, nil, &repos); err == nil {
		info.Usage = map[string]any{"repos_sample": len(repos)}
		for _, r := range repos {
			st := "public"
			if r.Private {
				st = "private"
			}
			info.Resources = append(info.Resources, Resource{ID: r.FullName, Name: r.FullName, Kind: "repo", URL: r.HTMLURL, Status: st})
		}
	}
	return info, nil
}

type gitlabProv struct{}

func (gitlabProv) Meta() Meta {
	return Meta{
		ID: "gitlab", Name: "GitLab", Auth: "token", Optional: true,
		Docs:         "https://docs.gitlab.com/ee/api/rest/",
		Capabilities: map[Capability]Support{CapImport: SupportYes, CapHosting: SupportNo},
		Fields: []Field{
			{Key: "token", Label: "Personal access token", Secret: true, Hint: "read_api or read_repository"},
			{Key: "base", Label: "GitLab URL", Optional: true, Placeholder: "https://gitlab.com", Hint: "Self-managed GitLab is fine"},
		},
		Scopes: []string{"read_repository (and read_api to list projects). No write scopes."},
		Notes:  "Optional import only. No runtime dependency.",
	}
}

func (gitlabProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	if err := require(creds, "token"); err != nil {
		return nil, err
	}
	official := strings.TrimSpace(creds["base"])
	if official == "" {
		official = "https://gitlab.com"
	}
	base := tr.BaseURL("gitlab", official)
	hdr := map[string]string{"PRIVATE-TOKEN": strings.TrimSpace(creds["token"])}
	var user struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/api/v4/user", hdr, nil, &user); err != nil {
		return nil, err
	}
	info := &Info{Who: firstNonEmpty(user.Name, user.Username), Health: "ok", Status: StatusConnected}
	var proj []struct {
		PathWithNamespace string `json:"path_with_namespace"`
		WebURL            string `json:"web_url"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/api/v4/projects?membership=true&per_page=20&order_by=updated_at", hdr, nil, &proj); err == nil {
		info.Usage = map[string]any{"projects_sample": len(proj)}
		for _, p := range proj {
			info.Resources = append(info.Resources, Resource{ID: p.PathWithNamespace, Name: p.PathWithNamespace, Kind: "project", URL: p.WebURL})
		}
	}
	return info, nil
}
