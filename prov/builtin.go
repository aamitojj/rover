package prov

import (
	"context"
	"strings"
)

func init() {
	Register(coolifyProv{})
	Register(nginxProv{})
	Register(localProv{})
}

type coolifyProv struct{}

func (coolifyProv) Meta() Meta {
	return Meta{
		ID: "coolify", Name: "Coolify", Auth: "token", Existing: true,
		Docs: "https://coolify.io/docs/api-reference/api/operations/version",
		Capabilities: map[Capability]Support{
			CapHosting: SupportYes, CapMonitoring: SupportPartial,
		},
		Fields: []Field{
			{Key: "url", Label: "Coolify URL", Placeholder: "http://203.0.113.10:8000"},
			{Key: "token", Label: "API token", Secret: true, Hint: "Keys & Tokens in Coolify; deploy permission is required"},
		},
		Scopes: []string{"A Coolify API token with permission to list and deploy applications (or a root token)."},
		Notes:  "Already built into Rover. Connecting here is optional: the token on the active server is used automatically.",
	}
}

func (coolifyProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	if err := require(creds, "url", "token"); err != nil {
		return nil, err
	}
	base := tr.BaseURL("coolify", strings.TrimRight(creds["url"], "/"))
	base = strings.TrimSuffix(base, "/api/v1")
	var ver struct {
		Version string `json:"version"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/api/v1/version", bearer(creds["token"]), nil, &ver); err != nil {
		return nil, err
	}
	var apps []struct {
		UUID   string `json:"uuid"`
		Name   string `json:"name"`
		FQDN   string `json:"fqdn"`
		Status string `json:"status"`
	}
	info := &Info{Who: "Coolify " + ver.Version, Plan: ver.Version, Health: "ok", Status: StatusConnected}
	if err := tr.DoJSON(ctx, "GET", base+"/api/v1/applications", bearer(creds["token"]), nil, &apps); err == nil {
		for _, a := range apps {
			info.Resources = append(info.Resources, Resource{ID: a.UUID, Name: a.Name, Kind: "application", URL: a.FQDN, Status: a.Status})
		}
		info.Usage = map[string]any{"applications": len(apps)}
	}
	return info, nil
}

type nginxProv struct{}

func (nginxProv) Meta() Meta {
	return Meta{
		ID: "nginx", Name: "Nginx direct", Auth: "ssh", Existing: true,
		Docs:         "https://nginx.org/en/docs/",
		Capabilities: map[Capability]Support{CapHosting: SupportYes, CapMonitoring: SupportYes},
		Notes:        "Installs and drives nginx over SSH on Ubuntu, Debian or Oracle Linux (amd64 and arm64), tuned for 1 GB RAM. No Docker or Coolify. Choose it as the engine when you add a server.",
	}
}

func (nginxProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	// Connectivity is proven over SSH in the main app, not here.
	return &Info{Status: StatusConnected, Health: "ok", Who: "nginx on the server (tested over SSH)"}, nil
}

type localProv struct{}

func (localProv) Meta() Meta {
	return Meta{
		ID: "local", Name: "This PC", Auth: "local", Existing: true,
		Capabilities: map[Capability]Support{CapHosting: SupportYes, CapMonitoring: SupportYes},
		Notes:        "The built-in web server on this Windows PC. Already in Rover (This PC). No cloud account.",
	}
}

func (localProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	return &Info{Status: StatusConnected, Health: "ok", Who: "this PC"}, nil
}
