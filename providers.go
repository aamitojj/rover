package main

import (
	"sort"

	"hostpanel/prov"
)

// ProviderDefinition describes capabilities, not a live connection.
// Credentials and account bindings must be managed separately.
type ProviderDefinition struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Capabilities   []string `json:"capabilities"`
	ExistingEngine bool     `json:"existing_engine"`
}

func providerCatalog() []ProviderDefinition {
	var out []ProviderDefinition
	for _, m := range prov.Catalog() {
		var caps []string
		for c, s := range m.Capabilities {
			if s != prov.SupportNo {
				caps = append(caps, string(c))
			}
		}
		sort.Strings(caps)
		out = append(out, ProviderDefinition{ID: m.ID, Name: m.Name, Capabilities: caps, ExistingEngine: m.Existing})
	}
	return out
}

func providerSupports(id, capability string) bool {
	return prov.Supports(id, prov.Capability(capability))
}
