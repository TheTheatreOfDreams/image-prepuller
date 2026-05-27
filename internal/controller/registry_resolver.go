package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const (
	InsecureSkipVerifyRegistriesEnv = "PREPULLER_INSECURE_SKIP_VERIFY_REGISTRIES"
	TargetPlatformsEnv              = "PREPULLER_PLATFORMS"
)

type ImagePlatformResolver interface {
	ResolvePlatforms(ctx context.Context, reference string) (map[string]string, error)
}

type RegistryResolver struct {
	InsecureSkipVerifyRegistries map[string]struct{}
	TargetPlatforms              map[string]struct{}
}

func RegistryResolverFromEnv() RegistryResolver {
	return RegistryResolverFromConfig(
		os.Getenv(TargetPlatformsEnv),
		os.Getenv(InsecureSkipVerifyRegistriesEnv),
	)
}

func RegistryResolverFromConfig(targetPlatforms, insecureSkipVerifyRegistries string) RegistryResolver {
	return RegistryResolver{
		InsecureSkipVerifyRegistries: parseSet(insecureSkipVerifyRegistries),
		TargetPlatforms:              parseSet(targetPlatforms),
	}
}

func (r RegistryResolver) ResolvePlatforms(ctx context.Context, reference string) (map[string]string, error) {
	ref, err := name.ParseReference(reference)
	if err != nil {
		return nil, fmt.Errorf("parse image reference %q: %w", reference, err)
	}

	descriptor, err := remote.Get(ref, r.remoteOptions(ctx, ref)...)
	if err != nil {
		return nil, fmt.Errorf("get image descriptor for %q: %w", reference, err)
	}

	switch {
	case descriptor.MediaType.IsIndex():
		index, err := descriptor.ImageIndex()
		if err != nil {
			return nil, fmt.Errorf("load image index for %q: %w", reference, err)
		}
		return r.platformsFromIndex(ref, index)
	case descriptor.MediaType.IsImage():
		image, err := descriptor.Image()
		if err != nil {
			return nil, fmt.Errorf("load image manifest for %q: %w", reference, err)
		}
		return r.platformFromImage(ref, image)
	default:
		return nil, fmt.Errorf("unsupported media type %q for %q", descriptor.MediaType, reference)
	}
}

func (r RegistryResolver) remoteOptions(ctx context.Context, ref name.Reference) []remote.Option {
	options := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
	}
	if _, ok := r.InsecureSkipVerifyRegistries[ref.Context().RegistryStr()]; ok {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		// This is intentionally opt-in for local/dev clusters behind TLS-intercepting proxies.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		options = append(options, remote.WithTransport(transport))
	}
	return options
}

func (r RegistryResolver) platformsFromIndex(ref name.Reference, index v1.ImageIndex) (map[string]string, error) {
	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("read image index manifest: %w", err)
	}

	platforms := make(map[string]string, len(manifest.Manifests))
	for _, descriptor := range manifest.Manifests {
		if !isRunnablePlatform(descriptor.Platform) {
			continue
		}
		key := platformKey(*descriptor.Platform)
		if !r.wantsPlatform(key) {
			continue
		}
		platforms[key] = pinnedReference(ref, descriptor.Digest)
	}
	return platforms, nil
}

func (r RegistryResolver) platformFromImage(ref name.Reference, image v1.Image) (map[string]string, error) {
	config, err := image.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("read image config: %w", err)
	}
	digest, err := image.Digest()
	if err != nil {
		return nil, fmt.Errorf("read image digest: %w", err)
	}

	platform := v1.Platform{
		OS:           config.OS,
		Architecture: config.Architecture,
		Variant:      config.Variant,
	}
	if !isRunnablePlatform(&platform) {
		return map[string]string{}, nil
	}
	key := platformKey(platform)
	if !r.wantsPlatform(key) {
		return map[string]string{}, nil
	}
	return map[string]string{key: pinnedReference(ref, digest)}, nil
}

func (r RegistryResolver) wantsPlatform(platform string) bool {
	if len(r.TargetPlatforms) == 0 {
		return true
	}
	_, ok := r.TargetPlatforms[platform]
	return ok
}

func isRunnablePlatform(platform *v1.Platform) bool {
	return platform != nil &&
		platform.OS != "" &&
		platform.Architecture != "" &&
		platform.OS != "unknown" &&
		platform.Architecture != "unknown"
}

func platformKey(platform v1.Platform) string {
	key := platform.OS + "/" + platform.Architecture
	if platform.Variant != "" {
		key += "/" + platform.Variant
	}
	return key
}

func pinnedReference(ref name.Reference, digest v1.Hash) string {
	return ref.Context().Digest(digest.String()).String()
}

func parseSet(value string) map[string]struct{} {
	items := map[string]struct{}{}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		items[item] = struct{}{}
	}
	return items
}
