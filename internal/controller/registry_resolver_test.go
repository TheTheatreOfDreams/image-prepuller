package controller

import (
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func TestParseRegistrySet(t *testing.T) {
	registries := parseSet("registry.k8s.io, ghcr.io, registry.k8s.io,")

	for _, registry := range []string{"registry.k8s.io", "ghcr.io"} {
		if _, ok := registries[registry]; !ok {
			t.Fatalf("expected %q to be parsed from registry set", registry)
		}
	}
	if _, ok := registries[""]; ok {
		t.Fatal("did not expect empty registry entry")
	}
}

func TestRegistryResolverWantsPlatform(t *testing.T) {
	resolver := RegistryResolverFromConfig("linux/amd64, linux/arm64", "")

	if !resolver.wantsPlatform("linux/amd64") {
		t.Fatal("expected linux/amd64 to be wanted")
	}
	if !resolver.wantsPlatform("linux/arm64") {
		t.Fatal("expected linux/arm64 to be wanted")
	}
	if resolver.wantsPlatform("linux/arm/v7") {
		t.Fatal("did not expect linux/arm/v7 to be wanted")
	}
}

func TestRegistryResolverWantsAllPlatformsWhenUnset(t *testing.T) {
	resolver := RegistryResolverFromConfig("", "")

	if !resolver.wantsPlatform("linux/amd64") {
		t.Fatal("expected unset target platforms to allow linux/amd64")
	}
	if !resolver.wantsPlatform("linux/arm/v7") {
		t.Fatal("expected unset target platforms to allow linux/arm/v7")
	}
}

func TestIsRunnablePlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform *v1.Platform
		want     bool
	}{
		{
			name:     "nil",
			platform: nil,
			want:     false,
		},
		{
			name:     "unknown",
			platform: &v1.Platform{OS: "unknown", Architecture: "unknown"},
			want:     false,
		},
		{
			name:     "missing architecture",
			platform: &v1.Platform{OS: "linux"},
			want:     false,
		},
		{
			name:     "linux arm64",
			platform: &v1.Platform{OS: "linux", Architecture: "arm64"},
			want:     true,
		},
		{
			name:     "linux arm v7",
			platform: &v1.Platform{OS: "linux", Architecture: "arm", Variant: "v7"},
			want:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isRunnablePlatform(test.platform); got != test.want {
				t.Fatalf("expected %v, got %v", test.want, got)
			}
		})
	}
}
