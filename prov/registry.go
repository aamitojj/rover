package prov

import "sync"

var (
	regMu sync.RWMutex
	reg   = map[string]Provider{}
	order []string
)

// Register adds a provider to the global catalog. Called from init().
func Register(p Provider) {
	m := p.Meta()
	if m.ID == "" {
		panic("provider with empty id")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, ok := reg[m.ID]; ok {
		panic("duplicate provider " + m.ID)
	}
	reg[m.ID] = p
	order = append(order, m.ID)
}

// Lookup returns a registered provider.
func Lookup(id string) (Provider, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	p, ok := reg[id]
	return p, ok
}

// Catalog is the registered providers in registration order.
func Catalog() []Meta {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Meta, 0, len(order))
	for _, id := range order {
		out = append(out, reg[id].Meta())
	}
	return out
}

// Supports reports whether id declares capability as supported or partial.
func Supports(id string, c Capability) bool {
	p, ok := Lookup(id)
	if !ok {
		return false
	}
	return supports(p.Meta().Capabilities, c)
}

// IDs returns registered provider identifiers in order.
func IDs() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := append([]string(nil), order...)
	return out
}
