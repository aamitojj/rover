package prov

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

func init() {
	Register(ga4Prov{})
	Register(gscProv{})
	Register(adsenseProv{})
	Register(roverAnalyticsProv{})
}

const (
	scopeGA4     = "https://www.googleapis.com/auth/analytics.readonly"
	scopeGSC     = "https://www.googleapis.com/auth/webmasters.readonly"
	scopeAdSense = "https://www.googleapis.com/auth/adsense.readonly"
)

type ga4Prov struct{}

func (ga4Prov) Meta() Meta {
	return Meta{
		ID: "ga4", Name: "Google Analytics", Auth: "oauth",
		Docs:         "https://developers.google.com/analytics/devguides/reporting/data/v1",
		Capabilities: map[Capability]Support{CapAnalytics: SupportYes},
		Fields: []Field{
			{Key: "access_token", Label: "OAuth access token (from Sign in with Google)", Secret: true, Optional: true},
			{Key: "json", Label: "Or a service account JSON", Secret: true, Multiline: true, Optional: true},
			{Key: "property_id", Label: "GA4 property ID", Optional: true, Placeholder: "properties/123456789", Hint: "Leave blank to list properties"},
		},
		Scopes: []string{"analytics.readonly only."},
		Notes:  "Uses the Google Analytics Data API (GA4). Sign in with Google (loopback + PKCE) or a service account that has been added to the property. First-party Rover Analytics does not need this.",
	}
}

func (ga4Prov) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	hdr, err := googleBearer(ctx, tr, creds, scopeGA4)
	if err != nil {
		return nil, err
	}
	admin := tr.BaseURL("ga-admin", "https://analyticsadmin.googleapis.com")
	var list struct {
		Properties []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"properties"`
	}
	// Account summaries is the supported list call.
	var sums struct {
		AccountSummaries []struct {
			PropertySummaries []struct {
				Property    string `json:"property"`
				DisplayName string `json:"displayName"`
			} `json:"propertySummaries"`
		} `json:"accountSummaries"`
	}
	info := &Info{Health: "ok", Status: StatusConnected}
	if err := tr.DoJSON(ctx, "GET", admin+"/v1beta/accountSummaries?pageSize=200", hdr, nil, &sums); err == nil {
		for _, a := range sums.AccountSummaries {
			for _, p := range a.PropertySummaries {
				info.Resources = append(info.Resources, Resource{ID: p.Property, Name: p.DisplayName, Kind: "property"})
			}
		}
		info.Usage = map[string]any{"properties": len(info.Resources)}
		if len(info.Resources) > 0 {
			info.Who = info.Resources[0].Name
		}
	} else if err2 := tr.DoJSON(ctx, "GET", admin+"/v1beta/properties?pageSize=50", hdr, nil, &list); err2 == nil {
		for _, p := range list.Properties {
			info.Resources = append(info.Resources, Resource{ID: p.Name, Name: p.DisplayName, Kind: "property"})
		}
	} else {
		return nil, err
	}
	return info, nil
}

// GA4RunReport calls properties/{id}:runReport.
func GA4RunReport(ctx context.Context, tr *Transport, creds map[string]string, property, start, end string) (map[string]any, error) {
	hdr, err := googleBearer(ctx, tr, creds, scopeGA4)
	if err != nil {
		return nil, err
	}
	prop := property
	if !strings.HasPrefix(prop, "properties/") {
		prop = "properties/" + prop
	}
	base := tr.BaseURL("ga4-data", "https://analyticsdata.googleapis.com")
	body := map[string]any{
		"dateRanges": []any{map[string]string{"startDate": start, "endDate": end}},
		"metrics":    []any{map[string]string{"name": "activeUsers"}, map[string]string{"name": "sessions"}, map[string]string{"name": "screenPageViews"}},
		"dimensions": []any{map[string]string{"name": "date"}},
	}
	var out map[string]any
	if err := tr.DoJSON(ctx, "POST", base+"/v1beta/"+prop+":runReport", hdr, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

type gscProv struct{}

func (gscProv) Meta() Meta {
	return Meta{
		ID: "search-console", Name: "Google Search Console", Auth: "oauth",
		Docs:         "https://developers.google.com/webmaster-tools/v1/api_reference_index",
		Capabilities: map[Capability]Support{CapAnalytics: SupportYes},
		Fields: []Field{
			{Key: "access_token", Label: "OAuth access token", Secret: true, Optional: true},
			{Key: "json", Label: "Or a service account JSON", Secret: true, Multiline: true, Optional: true, Hint: "The service account must be added as a user in Search Console"},
		},
		Scopes: []string{"webmasters.readonly only."},
	}
}

func (gscProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	hdr, err := googleBearer(ctx, tr, creds, scopeGSC)
	if err != nil {
		return nil, err
	}
	base := tr.BaseURL("gsc", "https://searchconsole.googleapis.com")
	var out struct {
		SiteEntry []struct {
			SiteURL         string `json:"siteUrl"`
			PermissionLevel string `json:"permissionLevel"`
		} `json:"siteEntry"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/webmasters/v3/sites", hdr, nil, &out); err != nil {
		return nil, err
	}
	info := &Info{Health: "ok", Status: StatusConnected, Usage: map[string]any{"sites": len(out.SiteEntry)}}
	for _, s := range out.SiteEntry {
		info.Resources = append(info.Resources, Resource{ID: s.SiteURL, Name: s.SiteURL, Kind: "site", Status: s.PermissionLevel})
	}
	if len(info.Resources) > 0 {
		info.Who = info.Resources[0].Name
	}
	return info, nil
}

// GSCQuery runs a searchAnalytics query.
func GSCQuery(ctx context.Context, tr *Transport, creds map[string]string, siteURL, start, end string) (map[string]any, error) {
	hdr, err := googleBearer(ctx, tr, creds, scopeGSC)
	if err != nil {
		return nil, err
	}
	base := tr.BaseURL("gsc", "https://searchconsole.googleapis.com")
	var out map[string]any
	body := map[string]any{"startDate": start, "endDate": end, "dimensions": []string{"query", "page"}, "rowLimit": 25}
	u := base + "/webmasters/v3/sites/" + url.PathEscape(siteURL) + "/searchAnalytics/query"
	if err := tr.DoJSON(ctx, "POST", u, hdr, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

type adsenseProv struct{}

func (adsenseProv) Meta() Meta {
	return Meta{
		ID: "adsense", Name: "Google AdSense", Auth: "oauth",
		Docs:         "https://developers.google.com/adsense/management/reference/rest",
		Capabilities: map[Capability]Support{CapAnalytics: SupportPartial, CapCost: SupportNo},
		Fields: []Field{
			{Key: "access_token", Label: "OAuth access token", Secret: true, Optional: true},
			{Key: "json", Label: "Or a service account JSON", Secret: true, Multiline: true, Optional: true},
		},
		Scopes: []string{"adsense.readonly only. Rover never changes AdSense settings or payments."},
		Notes:  "Reports estimated earnings and impressions. AdSense has no write actions in Rover.",
	}
}

func (adsenseProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	hdr, err := googleBearer(ctx, tr, creds, scopeAdSense)
	if err != nil {
		return nil, err
	}
	base := tr.BaseURL("adsense", "https://adsense.googleapis.com")
	var out struct {
		Accounts []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			State       string `json:"state"`
		} `json:"accounts"`
	}
	if err := tr.DoJSON(ctx, "GET", base+"/v2/accounts", hdr, nil, &out); err != nil {
		return nil, err
	}
	info := &Info{Health: "ok", Status: StatusConnected, Usage: map[string]any{"accounts": len(out.Accounts)}}
	for _, a := range out.Accounts {
		info.Resources = append(info.Resources, Resource{ID: a.Name, Name: firstNonEmpty(a.DisplayName, a.Name), Kind: "account", Status: a.State})
	}
	if len(info.Resources) > 0 {
		info.Who = info.Resources[0].Name
	}
	return info, nil
}

// AdSenseReport generates an earnings report for an account (accounts/{id}).
func AdSenseReport(ctx context.Context, tr *Transport, creds map[string]string, account, start, end string) (map[string]any, error) {
	hdr, err := googleBearer(ctx, tr, creds, scopeAdSense)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(account, "accounts/") {
		account = "accounts/" + account
	}
	base := tr.BaseURL("adsense", "https://adsense.googleapis.com")
	u := fmt.Sprintf("%s/v2/%s/reports:generate?dateRange=CUSTOM&startDate.year=%s&startDate.month=%s&startDate.day=%s&endDate.year=%s&endDate.month=%s&endDate.day=%s&metrics=ESTIMATED_EARNINGS&metrics=IMPRESSIONS&metrics=CLICKS&dimensions=DATE",
		base, account, start[:4], start[5:7], start[8:10], end[:4], end[5:7], end[8:10])
	var out map[string]any
	if err := tr.DoJSON(ctx, "GET", u, hdr, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

type roverAnalyticsProv struct{}

func (roverAnalyticsProv) Meta() Meta {
	return Meta{
		ID: "rover-analytics", Name: "Rover Analytics", Auth: "local", Existing: true,
		Capabilities: map[Capability]Support{CapAnalytics: SupportYes},
		Notes:        "First-party, privacy-friendly analytics from a tiny beacon and/or the site's nginx access log. No cookies. Estimated uniques are a daily salted hash. Country comes from the Cloudflare CF-IPCountry header when present, or from an optional GeoLite-style database you provide (MaxMind's licence does not allow Rover to bundle one). No Google account required.",
	}
}

func (roverAnalyticsProv) Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error) {
	return &Info{Status: StatusConnected, Health: "ok", Who: "first-party analytics on this PC"}, nil
}
