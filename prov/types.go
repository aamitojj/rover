// Package prov is Rover's modular multi-cloud provider layer.
//
// A new platform is added by implementing Provider and registering it in
// init(). Each provider declares its capabilities and whether each one is
// supported, partial or unsupported (never faked). Credentials are passed
// in; this package never stores them. HTTP base URLs are overridable so
// tests can drive official request shapes against httptest servers.
package prov

import (
	"context"
	"sort"
	"strings"
)

// Capability is one kind of work a provider can do.
type Capability string

const (
	CapHosting    Capability = "hosting"
	CapDNS        Capability = "dns"
	CapDatabase   Capability = "database"
	CapAuth       Capability = "auth"
	CapStorage    Capability = "storage"
	CapAnalytics  Capability = "analytics"
	CapMonitoring Capability = "monitoring"
	CapCost       Capability = "cost"
	CapImport     Capability = "import" // optional source import; no runtime dependency
	CapTunnels    Capability = "tunnels"
)

// Support says whether a capability is really available through a public API.
type Support string

const (
	SupportYes     Support = "supported"
	SupportPartial Support = "partial"
	SupportNo      Support = "unsupported"
)

// Status is a connection state shown in the Integration Centre.
type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnected    Status = "connected"
	StatusFailed       Status = "failed"
	StatusUnsupported  Status = "unsupported"
)

// Field is one credential field the user fills in.
type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Secret      bool     `json:"secret,omitempty"`
	Multiline   bool     `json:"multiline,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Hint        string   `json:"hint,omitempty"`
	Optional    bool     `json:"optional,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// Meta is the catalog entry for a provider (no live credentials).
type Meta struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Docs         string                 `json:"docs,omitempty"`
	Auth         string                 `json:"auth"` // token | keys | oauth | service_account | local | ssh
	Capabilities map[Capability]Support `json:"capabilities"`
	Fields       []Field                `json:"fields,omitempty"`
	Scopes       []string               `json:"scopes,omitempty"` // least-privilege, documented
	Notes        string                 `json:"notes,omitempty"`
	Existing     bool                   `json:"existing_engine,omitempty"`
	Optional     bool                   `json:"optional,omitempty"`
	PaidWarning  string                 `json:"paid_warning,omitempty"` // shown before a paid API is used
}

// Info is the result of a live Test against the provider API.
type Info struct {
	Who       string         `json:"who,omitempty"`
	Plan      string         `json:"plan,omitempty"`
	Health    string         `json:"health,omitempty"` // ok | degraded | unknown
	Usage     map[string]any `json:"usage,omitempty"`
	Resources []Resource     `json:"resources,omitempty"`
	Status    Status         `json:"status"`
}

// Resource is one project, site, zone, bucket, server, property, etc.
type Resource struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind,omitempty"`
	Region string `json:"region,omitempty"`
	URL    string `json:"url,omitempty"`
	Status string `json:"status,omitempty"`
	Extra  string `json:"extra,omitempty"`
}

// CostReport is a read-only spend summary. Some APIs charge for the query
// itself; callers must obtain explicit confirmation first (see PaidWarning).
type CostReport struct {
	Currency string     `json:"currency"`
	Total    string     `json:"total"`
	Period   string     `json:"period"`
	Lines    []CostLine `json:"lines,omitempty"`
	Note     string     `json:"note,omitempty"`
}

// CostLine is one service or region in a cost report.
type CostLine struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
}

// Provider is one platform. Test must use the official API (or a mocked
// base URL that speaks the same protocol). Unimplemented capabilities
// return SupportNo and must not be pretended to work.
type Provider interface {
	Meta() Meta
	Test(ctx context.Context, tr *Transport, creds map[string]string) (*Info, error)
}

// Coster is implemented by providers that can report spend (read-only).
type Coster interface {
	Cost(ctx context.Context, tr *Transport, creds map[string]string, confirmPaid bool) (*CostReport, error)
}

func capList(m map[Capability]Support) []string {
	var out []string
	for c, s := range m {
		if s != SupportNo {
			out = append(out, string(c))
		}
	}
	sort.Strings(out)
	return out
}

func supports(m map[Capability]Support, c Capability) bool {
	s, ok := m[c]
	return ok && s != SupportNo
}

func require(creds map[string]string, keys ...string) error {
	for _, k := range keys {
		if strings.TrimSpace(creds[k]) == "" {
			return missing(k)
		}
	}
	return nil
}
