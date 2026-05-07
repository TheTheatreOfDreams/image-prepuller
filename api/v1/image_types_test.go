package v1

import "testing"

func TestImageSpecDeepCopyCopiesPlatforms(t *testing.T) {
	image := &Image{
		Spec: ImageSpec{
			Reference: "nginx:1.27",
			Platforms: map[string]string{
				"linux/amd64": "nginx@sha256:amd64",
				"linux/arm64": "nginx@sha256:arm64",
			},
		},
	}

	copied := image.DeepCopy()
	copied.Spec.Platforms["linux/arm64"] = "nginx@sha256:changed"

	if image.Spec.Platforms["linux/arm64"] != "nginx@sha256:arm64" {
		t.Fatal("expected platform map to be deep-copied")
	}
}
