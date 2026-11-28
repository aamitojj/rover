package main

// ProviderDefinition describes capabilities, not a live connection.
// Credentials and account bindings must be managed separately.
type ProviderDefinition struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Capabilities []string `json:"capabilities"`
 ExistingEngine bool `json:"existing_engine"`
}

func providerCatalog() []ProviderDefinition {
 return []ProviderDefinition{
  {"nginx", "Nginx Direct", []string{"hosting"}, true},
  {"coolify", "Coolify", []string{"hosting"}, true},
  {"local", "Local PC", []string{"hosting", "testing"}, true},
  {"supabase", "Supabase", []string{"database", "auth", "storage"}, false},
  {"firebase", "Firebase", []string{"database", "auth", "notifications"}, false},
  {"netlify", "Netlify", []string{"hosting"}, false},
  {"vercel", "Vercel", []string{"hosting"}, false},
  {"oracle", "Oracle Cloud", []string{"infrastructure"}, false},
  {"gcp", "Google Cloud", []string{"infrastructure"}, false},
  {"aws", "AWS", []string{"infrastructure"}, false},
  {"azure", "Azure", []string{"infrastructure"}, false},
  {"cloudflare", "Cloudflare", []string{"dns", "cdn", "tunnels"}, false},
  {"github", "GitHub", []string{"source", "releases"}, false},
  {"gitlab", "GitLab", []string{"source"}, false},
  {"ga4", "Google Analytics", []string{"analytics"}, false},
  {"search-console", "Google Search Console", []string{"seo"}, false},
  {"adsense", "Google AdSense", []string{"revenue"}, false},
  {"rover-analytics", "Rover Analytics", []string{"analytics"}, false},
 }
}

func providerSupports(id, capability string) bool {
 for _, p := range providerCatalog() {
  if p.ID != id { continue }
  for _, c := range p.Capabilities { if c == capability { return true } }
 }
 return false
}
