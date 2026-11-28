package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hostpanel/prov"
)

func apiAnalyticsGoogle(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	c := snapshotConfig()
	s := findSite(&c, slug)
	if s == nil {
		writeErr(w, fmt.Errorf("unknown project"))
		return
	}
	days := 28
	if q := strings.TrimSpace(r.URL.Query().Get("days")); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 400 {
			days = n
		}
	}
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -(days - 1))
	if q := strings.TrimSpace(r.URL.Query().Get("start")); len(q) == 10 {
		if t, err := time.Parse("2006-01-02", q); err == nil {
			start = t
		}
	}
	if q := strings.TrimSpace(r.URL.Query().Get("end")); len(q) == 10 {
		if t, err := time.Parse("2006-01-02", q); err == nil {
			end = t
		}
	}
	startS, endS := start.Format("2006-01-02"), end.Format("2006-01-02")
	want := r.URL.Query().Get("source") // empty = all
	tr := integTransport()
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	out := map[string]any{"start": startS, "end": endS, "days": days}
	connected := false
	if want == "" || want == "ga4" {
		rep := fetchGA4Report(ctx, tr, &c, s, startS, endS)
		out["ga4"] = rep
		if rep["connected"] == true {
			connected = true
		}
	}
	if want == "" || want == "search-console" {
		rep := fetchGSCReport(ctx, tr, &c, s, startS, endS)
		out["search_console"] = rep
		if rep["connected"] == true {
			connected = true
		}
	}
	if want == "" || want == "adsense" {
		rep := fetchAdSenseReport(ctx, tr, &c, s, startS, endS)
		out["adsense"] = rep
		if rep["connected"] == true {
			connected = true
		}
	}
	out["connected"] = connected
	writeJSON(w, 200, out)
}

func siteServiceID(s *Site, keys ...string) string {
	if s == nil || s.Services == nil {
		return ""
	}
	for _, k := range keys {
		if id := strings.TrimSpace(s.Services[k]); id != "" {
			return id
		}
	}
	return ""
}

func integrationForReport(c *Config, assignedID, provider string) *Integration {
	if assignedID != "" {
		if a := findIntegration(c, assignedID); a != nil {
			return a
		}
	}
	var found *Integration
	n := 0
	for _, a := range c.Integrations {
		if a != nil && a.Provider == provider && a.Status != "disconnected" {
			found = a
			n++
		}
	}
	if n == 1 {
		return found
	}
	return nil
}

func reportCreds(c *Config, a *Integration) map[string]string {
	cr := map[string]string{}
	if a != nil {
		cr = integCred(a)
	}
	if cr["access_token"] == "" && c != nil && c.GoogleAccount != nil {
		if tok := secret(c.GoogleAccount.AccessToken); tok != "" {
			cr["access_token"] = tok
		}
	}
	return cr
}

func reportNotConnected(kind string) map[string]any {
	return map[string]any{
		"connected": false,
		"error":     "connect " + kind + " in the Integration Centre, or assign it to this project",
	}
}

func fetchGA4Report(ctx context.Context, tr *prov.Transport, c *Config, s *Site, start, end string) map[string]any {
	a := integrationForReport(c, siteServiceID(s, "analytics", "ga4"), "ga4")
	if a == nil && (c.GoogleAccount == nil || secret(c.GoogleAccount.AccessToken) == "") {
		return reportNotConnected("Google Analytics")
	}
	cr := reportCreds(c, a)
	prop := firstNonEmpty(cr["property_id"], cr["property"])
	if prop == "" {
		return map[string]any{"connected": true, "provider": "ga4", "error": "set a GA4 property ID on the account (properties/123456789)"}
	}
	raw, err := prov.GA4RunReport(ctx, tr, cr, prop, start, end)
	if err != nil {
		return map[string]any{"connected": true, "provider": "ga4", "property": prop, "error": err.Error()}
	}
	return map[string]any{"connected": true, "provider": "ga4", "property": prop, "who": whoOf(a), "table": simplifyGA4(raw)}
}

func fetchGSCReport(ctx context.Context, tr *prov.Transport, c *Config, s *Site, start, end string) map[string]any {
	a := integrationForReport(c, siteServiceID(s, "seo", "search-console"), "search-console")
	if a == nil && (c.GoogleAccount == nil || secret(c.GoogleAccount.AccessToken) == "") {
		return reportNotConnected("Search Console")
	}
	cr := reportCreds(c, a)
	siteURL := firstNonEmpty(cr["site_url"], cr["site"])
	if siteURL == "" && s != nil && s.Domain != "" {
		siteURL = "https://" + s.Domain + "/"
	}
	if siteURL == "" {
		return map[string]any{"connected": true, "provider": "search-console", "error": "set a site URL on the Search Console account, or add a domain to this project"}
	}
	raw, err := prov.GSCQuery(ctx, tr, cr, siteURL, start, end)
	if err != nil {
		return map[string]any{"connected": true, "provider": "search-console", "site": siteURL, "error": err.Error()}
	}
	return map[string]any{"connected": true, "provider": "search-console", "site": siteURL, "who": whoOf(a), "table": simplifyGSC(raw)}
}

func fetchAdSenseReport(ctx context.Context, tr *prov.Transport, c *Config, s *Site, start, end string) map[string]any {
	a := integrationForReport(c, siteServiceID(s, "revenue", "adsense"), "adsense")
	if a == nil && (c.GoogleAccount == nil || secret(c.GoogleAccount.AccessToken) == "") {
		return reportNotConnected("AdSense")
	}
	cr := reportCreds(c, a)
	account := firstNonEmpty(cr["account"], cr["account_id"])
	if account == "" {
		return map[string]any{"connected": true, "provider": "adsense", "error": "set an AdSense account ID on the account (accounts/pub-…)"}
	}
	raw, err := prov.AdSenseReport(ctx, tr, cr, account, start, end)
	if err != nil {
		return map[string]any{"connected": true, "provider": "adsense", "account": account, "error": err.Error()}
	}
	return map[string]any{"connected": true, "provider": "adsense", "account": account, "who": whoOf(a), "table": simplifyAdSense(raw)}
}

func whoOf(a *Integration) string {
	if a == nil {
		return ""
	}
	return firstNonEmpty(a.Who, a.Nickname)
}

func simplifyGA4(raw map[string]any) map[string]any {
	headers := []string{}
	for _, h := range asMaps(raw["dimensionHeaders"]) {
		if n, _ := h["name"].(string); n != "" {
			headers = append(headers, n)
		}
	}
	for _, h := range asMaps(raw["metricHeaders"]) {
		if n, _ := h["name"].(string); n != "" {
			headers = append(headers, n)
		}
	}
	rows := [][]string{}
	for _, r := range asMaps(raw["rows"]) {
		row := []string{}
		for _, d := range asMaps(r["dimensionValues"]) {
			row = append(row, fmt.Sprint(d["value"]))
		}
		for _, m := range asMaps(r["metricValues"]) {
			row = append(row, fmt.Sprint(m["value"]))
		}
		rows = append(rows, row)
	}
	return map[string]any{"headers": headers, "rows": rows}
}

func simplifyGSC(raw map[string]any) map[string]any {
	headers := []string{"query", "page", "clicks", "impressions", "ctr", "position"}
	rows := [][]string{}
	for _, r := range asMaps(raw["rows"]) {
		keys, _ := r["keys"].([]any)
		q, p := "", ""
		if len(keys) > 0 {
			q = fmt.Sprint(keys[0])
		}
		if len(keys) > 1 {
			p = fmt.Sprint(keys[1])
		}
		rows = append(rows, []string{q, p, fmt.Sprint(r["clicks"]), fmt.Sprint(r["impressions"]), fmt.Sprint(r["ctr"]), fmt.Sprint(r["position"])})
	}
	return map[string]any{"headers": headers, "rows": rows}
}

func simplifyAdSense(raw map[string]any) map[string]any {
	headers := []string{}
	for _, h := range asMaps(raw["headers"]) {
		if n, _ := h["name"].(string); n != "" {
			headers = append(headers, n)
		}
	}
	if len(headers) == 0 {
		headers = []string{"DATE", "ESTIMATED_EARNINGS", "IMPRESSIONS", "CLICKS"}
	}
	rows := [][]string{}
	for _, r := range asMaps(raw["rows"]) {
		row := []string{}
		if cells := asMaps(r["cells"]); len(cells) > 0 {
			for _, c := range cells {
				row = append(row, fmt.Sprint(c["value"]))
			}
		} else if cells, ok := r["cells"].([]any); ok {
			for _, c := range cells {
				if m, ok := c.(map[string]any); ok {
					row = append(row, fmt.Sprint(m["value"]))
				} else {
					row = append(row, fmt.Sprint(c))
				}
			}
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	return map[string]any{"headers": headers, "rows": rows}
}

func asMaps(v any) []map[string]any {
	arr, _ := v.([]any)
	out := []map[string]any{}
	for _, el := range arr {
		if m, ok := el.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
