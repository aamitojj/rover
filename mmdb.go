package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

// Optional MaxMind-format city database (GeoLite2 City, DB-IP City Lite, etc.).
// The user supplies the file; Rover never bundles one (licence). The reader is
// pure Go (github.com/oschwald/maxminddb-golang) so CGO stays off.

type mmdbStatus struct {
	Path  string `json:"path,omitempty"`
	OK    bool   `json:"ok"`
	Type  string `json:"type,omitempty"`
	Built string `json:"built,omitempty"`
	IPVer uint   `json:"ip_version,omitempty"`
	Error string `json:"error,omitempty"`
	Note  string `json:"note,omitempty"`
}

var (
	geoMu   sync.Mutex
	geoDB   *maxminddb.Reader
	geoPath string
	geoMod  time.Time
)

func closeGeoLocked() {
	if geoDB != nil {
		_ = geoDB.Close()
		geoDB = nil
		geoPath = ""
		geoMod = time.Time{}
	}
}

func openGeo(path string) (*maxminddb.Reader, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("no geolocation database is configured")
	}
	if strings.ContainsRune(path, 0) {
		return nil, fmt.Errorf("the database path is not valid")
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("could not open the geolocation database: %w", err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("the geolocation path is a folder, not a .mmdb file")
	}
	geoMu.Lock()
	defer geoMu.Unlock()
	if geoDB != nil && geoPath == path && geoMod.Equal(st.ModTime()) {
		return geoDB, nil
	}
	closeGeoLocked()
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("that file is not a readable MaxMind/DB-IP database: %w", err)
	}
	geoDB, geoPath, geoMod = db, path, st.ModTime()
	return db, nil
}

func geoStatus(path string) mmdbStatus {
	note := "Rover does not bundle a geolocation database. Provide a MaxMind-format .mmdb you have a licence to use, for example DB-IP City Lite (CC-BY). Country can also come from the Cloudflare CF-IPCountry header."
	if strings.TrimSpace(path) == "" {
		return mmdbStatus{OK: false, Note: note}
	}
	db, err := openGeo(path)
	if err != nil {
		return mmdbStatus{Path: path, OK: false, Error: err.Error(), Note: note}
	}
	md := db.Metadata
	built := ""
	if md.BuildEpoch > 0 {
		built = time.Unix(int64(md.BuildEpoch), 0).UTC().Format(time.RFC3339)
	}
	return mmdbStatus{Path: path, OK: true, Type: md.DatabaseType, Built: built, IPVer: md.IPVersion, Note: note}
}

func lookupGeo(ipStr string) (country, region, city string) {
	c := snapshotConfig()
	if c.Analytics == nil || strings.TrimSpace(c.Analytics.MMDBPath) == "" {
		return "", "", ""
	}
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return "", "", ""
	}
	db, err := openGeo(c.Analytics.MMDBPath)
	if err != nil {
		return "", "", ""
	}
	var rec map[string]any
	if err := db.Lookup(ip, &rec); err != nil || rec == nil {
		return "", "", ""
	}
	return extractGeo(rec)
}

func extractGeo(rec map[string]any) (country, region, city string) {
	// GeoLite2 / DB-IP structured records.
	if m, _ := rec["country"].(map[string]any); m != nil {
		country = firstNonEmpty(asString(m["iso_code"]), nameEN(m))
	}
	if country == "" {
		country = firstNonEmpty(asString(rec["country_code"]), asString(rec["countryCode"]), asString(rec["country"]))
	}
	if subs, _ := rec["subdivisions"].([]any); len(subs) > 0 {
		if m, ok := subs[0].(map[string]any); ok {
			region = firstNonEmpty(asString(m["iso_code"]), nameEN(m))
		}
	}
	if region == "" {
		region = firstNonEmpty(asString(rec["stateprov"]), asString(rec["subdivision"]), asString(rec["region"]))
	}
	if m, _ := rec["city"].(map[string]any); m != nil {
		city = nameEN(m)
	}
	if city == "" {
		city = firstNonEmpty(asString(rec["city"]), asString(rec["town"]))
	}
	return strings.ToUpper(strings.TrimSpace(country)), strings.TrimSpace(region), strings.TrimSpace(city)
}

func nameEN(m map[string]any) string {
	if m == nil {
		return ""
	}
	if names, _ := m["names"].(map[string]any); names != nil {
		return firstNonEmpty(asString(names["en"]), asString(names["en-US"]))
	}
	return ""
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func geoFromRequest(r *http.Request) (country, region, city string) {
	header := countryOf(r)
	cc, rg, ct := lookupGeo(clientIP(r))
	if cc != "" {
		return cc, rg, ct
	}
	return header, "", ""
}

func registerAnalyticsSettingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/analytics/settings", apiGetAnalyticsSettings)
	mux.HandleFunc("POST /api/analytics/settings", apiSaveAnalyticsSettings)
}

func apiGetAnalyticsSettings(w http.ResponseWriter, r *http.Request) {
	c := snapshotConfig()
	a := AnalyticsSettings{Enabled: true, RetentionDays: 90, BotFilter: true}
	if c.Analytics != nil {
		a = *c.Analytics
		a.Salt = ""
	}
	writeJSON(w, 200, map[string]any{"settings": a, "mmdb": geoStatus(a.MMDBPath)})
}

func apiSaveAnalyticsSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled       *bool  `json:"enabled"`
		RetentionDays int    `json:"retention_days"`
		BotFilter     *bool  `json:"bot_filter"`
		MMDBPath      string `json:"mmdb_path"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.MMDBPath != "" {
		if !filepath.IsAbs(req.MMDBPath) && !filepath.IsLocal(req.MMDBPath) {
			// Allow absolute paths (the usual case on the user's PC) and
			// simple relative names; reject ".." escapes.
			if strings.Contains(req.MMDBPath, "..") {
				writeErr(w, fmt.Errorf("the database path is not valid"))
				return
			}
		}
		if st := geoStatus(req.MMDBPath); !st.OK {
			writeErr(w, fmt.Errorf("%s", st.Error))
			return
		}
	} else {
		geoMu.Lock()
		closeGeoLocked()
		geoMu.Unlock()
	}
	if err := withConfig(func(c *Config) error {
		if c.Analytics == nil {
			c.Analytics = &AnalyticsSettings{Enabled: true, RetentionDays: 90, BotFilter: true}
		}
		if req.Enabled != nil {
			c.Analytics.Enabled = *req.Enabled
		}
		if req.BotFilter != nil {
			c.Analytics.BotFilter = *req.BotFilter
		}
		if req.RetentionDays > 0 {
			c.Analytics.RetentionDays = req.RetentionDays
		}
		c.Analytics.MMDBPath = strings.TrimSpace(req.MMDBPath)
		return nil
	}); err != nil {
		writeErr(w, err)
		return
	}
	c := snapshotConfig()
	a := *c.Analytics
	a.Salt = ""
	writeJSON(w, 200, map[string]any{"settings": a, "mmdb": geoStatus(a.MMDBPath)})
}
