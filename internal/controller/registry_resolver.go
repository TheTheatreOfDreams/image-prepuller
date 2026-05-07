package controller

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type ImagePlatformResolver interface {
	ResolvePlatforms(ctx context.Context, reference string) (map[string]string, error)
}

type RegistryResolver struct{}

func (RegistryResolver) ResolvePlatforms(ctx context.Context, reference string) (map[string]string, error) {
	ref, err := name.ParseReference(reference)
	if err != nil {
		return nil, fmt.Errorf("parse image reference %q: %w", reference, err)
	}

	descriptor, err := remote.Get(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("get image descriptor for %q: %w", reference, err)
	}

	switch {
	case descriptor.MediaType.IsIndex():
		index, err := descriptor.ImageIndex()
		if err != nil {
			return nil, fmt.Errorf("load image index for %q: %w", reference, err)
		}
		return platformsFromIndex(ref, index)
	case descriptor.MediaType.IsImage():
		image, err := descriptor.Image()
		if err != nil {
			return nil, fmt.Errorf("load image manifest for %q: %w", reference, err)
		}
		return platformFromImage(ref, image)
	default:
		return nil, fmt.Errorf("unsupported media type %q for %q", descriptor.MediaType, reference)
	}
}

func platformsFromIndex(ref name.Reference, index v1.ImageIndex) (map[string]string, error) {
	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("read image index manifest: %w", err)
	}

	platforms := make(map[string]string, len(manifest.Manifests))
	for _, descriptor := range manifest.Manifests {
		if descriptor.Platform == nil {
			continue
		}
		platforms[platformKey(*descriptor.Platform)] = pinnedReference(ref, descriptor.Digest)
	}
	return platforms, nil
}

func platformFromImage(ref name.Reference, image v1.Image) (map[string]string, error) {
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
	if platform.OS == "" || platform.Architecture == "" {
		return map[string]string{"unknown/unknown": pinnedReference(ref, digest)}, nil
	}
	return map[string]string{platformKey(platform): pinnedReference(ref, digest)}, nil
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
