package main

import "testing"

func TestProviderCatalog(t *testing.T) {
 seen := map[string]bool{}
 for _, p := range providerCatalog() {
  if p.ID == "" || p.Name == "" || len(p.Capabilities) == 0 || seen[p.ID] {
   t.Fatalf("invalid or duplicate provider: %+v", p)
  }
  seen[p.ID] = true
 }
 if !providerSupports("supabase", "auth") || !providerSupports("coolify", "hosting") || providerSupports("supabase", "hosting") || providerSupports("unknown", "hosting") {
  t.Fatal("incorrect provider capability resolution")
 }
}
