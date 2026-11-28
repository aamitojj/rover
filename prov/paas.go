package prov

import (
	"bytes"
	"context"
	"fmt"
	"strings"
)

func init() {
	Register(netlifyProv{})
	Register(vercelProv{})
}

type netlifyProv struct{}

func (netlifyProv) Meta() Meta {
	return Meta{
		ID: "netlify", Name: "Netlify", Auth: "token",
		Docs:         "https://docs.netlify.com/api/get-started/",
		Capabilities: map[Capability]Support{CapHosting: SupportYes, CapAnalytics: SupportNo, CapDNS: SupportPartial},
		Fields:       []Field{{Key: "token", Label: "Personal access token", Secret: true, Hint: "User settings → Applications → New access token"}},
		Scopes:       []string{"A personal access token (no finer scope exists; treat it as full account access)."},
		Notes:        "List sites, deploys and upload a ZIP. Netlify Analytics is a paid add-on with no public API Rover will call.",
	}
}

func (netlifyProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	if err := require(creds, "token"); err != nil {
		return nil, err
	}
	base := tr.BaseURL("netlify", "https://api.netlify.com")
	var user struct {
		ID        string `json:"id"`
		Email     string `json:"email"`
		Name      string `json:"full_name"`
		Slug      string `json:"slug"`
		SiteCount int    `json:"site_count"`
		AccountID string `json:"account_id"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/api/v1/user", bearer(creds["token"]), nil, &user); err != nil {
		return nil, err
	}
	who := firstNonEmpty(user.Name, user.Email, user.Slug, user.ID)
	info := &Info{Who: who, Health: "ok", Status: StatusConnected, Usage: map[string]any{"sites": user.SiteCount}}
	var sites []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		URL   string `json:"url"`
		SSL   bool   `json:"ssl"`
		State string `json:"state"`
		Plan  string `json:"plan_name"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/api/v1/sites?per_page=100", bearer(creds["token"]), nil, &sites); err == nil {
		info.Usage["sites"] = len(sites)
		for _, s := range sites {
			if info.Plan == "" && s.Plan != "" {
				info.Plan = s.Plan
			}
			info.Resources = append(info.Resources, Resource{ID: s.ID, Name: s.Name, Kind: "site", URL: s.URL, Status: s.State})
		}
	}
	return info, nil
}

// NetlifyDeploy uploads a zip (application/zip) as a new deploy for a site.
func NetlifyDeploy(ctx context.Context, tr *Transport, creds map[string]string, siteID string, zip []byte) (string, error) {
	if err := require(creds, "token"); err != nil {
		return "", err
	}
	if siteID == "" {
		return "", fmt.Errorf("choose a Netlify site")
	}
	base := tr.BaseURL("netlify", "https://api.netlify.com")
	var out struct {
		ID     string `json:"id"`
		State  string `json:"state"`
		SSLURL string `json:"ssl_url"`
		URL    string `json:"url"`
	}
	hdr := bearer(creds["token"])
	hdr["Content-Type"] = "application/zip"
	if err := tr.Do(ctx, "POST", base+"/api/v1/sites/"+siteID+"/deploys", hdr, bytes.NewReader(zip), &out); err != nil {
		return "", err
	}
	return firstNonEmpty(out.SSLURL, out.URL, out.ID), nil
}

type vercelProv struct{}

func (vercelProv) Meta() Meta {
	return Meta{
		ID: "vercel", Name: "Vercel", Auth: "token",
		Docs:         "https://vercel.com/docs/rest-api",
		Capabilities: map[Capability]Support{CapHosting: SupportYes, CapDNS: SupportPartial, CapAnalytics: SupportNo},
		Fields: []Field{
			{Key: "token", Label: "Access token", Secret: true, Hint: "Vercel → Settings → Tokens"},
			{Key: "team_id", Label: "Team ID", Optional: true, Hint: "Only if the projects live on a team"},
		},
		Scopes: []string{"A token with scope: user (read), project (read), deployment (read/create if you deploy)."},
		Notes:  "Web Analytics and Speed Insights have no general-purpose export API Rover can use; they are marked unsupported.",
	}
}

func (vercelProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	if err := require(creds, "token"); err != nil {
		return nil, err
	}
	base := tr.BaseURL("vercel", "https://api.vercel.com")
	q := ""
	if t := strings.TrimSpace(creds["team_id"]); t != "" {
		q = "?teamId=" + t
	}
	var user struct {
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Name     string `json:"name"`
			Email    string `json:"email"`
		} `json:"user"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/v2/user"+q, bearer(creds["token"]), nil, &user); err != nil {
		return nil, err
	}
	info := &Info{Who: firstNonEmpty(user.User.Name, user.User.Username, user.User.Email), Health: "ok", Status: StatusConnected}
	var proj struct {
		Projects []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Frame string `json:"framework"`
			Link  any    `json:"link"`
		} `json:"projects"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/v9/projects?limit=100"+strings.Replace(q, "?", "&", 1), bearer(creds["token"]), nil, &proj); err == nil {
		info.Usage = map[string]any{"projects": len(proj.Projects)}
		for _, p := range proj.Projects {
			info.Resources = append(info.Resources, Resource{ID: p.ID, Name: p.Name, Kind: "project", Extra: p.Frame})
		}
	}
	return info, nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
