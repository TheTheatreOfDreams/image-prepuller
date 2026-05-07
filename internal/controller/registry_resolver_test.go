package controller

import "testing"

func TestParseRegistrySet(t *testing.T) {
	registries := parseRegistrySet("registry.k8s.io, ghcr.io, registry.k8s.io,")

	for _, registry := range []string{"registry.k8s.io", "ghcr.io"} {
		if _, ok := registries[registry]; !ok {
			t.Fatalf("expected %q to be parsed from registry set", registry)
		}
	}
	if _, ok := registries[""]; ok {
		t.Fatal("did not expect empty registry entry")
	}
}
