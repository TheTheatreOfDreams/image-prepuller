package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	imagev1 "github.com/TheTheatreOfDreams/image-prepuller/api/v1"
)

type fakePlatformResolver struct {
	platforms map[string]map[string]string
	err       error
}

func (f fakePlatformResolver) ResolvePlatforms(_ context.Context, reference string) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	platforms, ok := f.platforms[reference]
	if !ok {
		return nil, fmt.Errorf("unexpected image reference %q", reference)
	}
	return platforms, nil
}

func TestImageReferencesFromPod(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "init", Image: "busybox:1.36"},
			},
			Containers: []corev1.Container{
				{Name: "app", Image: "nginx:1.27"},
				{Name: "sidecar", Image: "busybox:1.36"},
				{Name: "empty"},
			},
			EphemeralContainers: []corev1.EphemeralContainer{
				{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debug", Image: "alpine:3.20"}},
			},
		},
	}

	got := imageReferencesFromPod(pod)
	want := []string{"alpine:3.20", "busybox:1.36", "nginx:1.27"}

	if len(got) != len(want) {
		t.Fatalf("expected %d references, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("reference %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestImageResourceName(t *testing.T) {
	name := imageResourceName("registry.example.com/team/app:1.0.0")

	if len(name) != 44 {
		t.Fatalf("expected name length 44, got %d", len(name))
	}
	if name != imageResourceName("registry.example.com/team/app:1.0.0") {
		t.Fatal("expected image resource name to be stable")
	}
	if name == imageResourceName("registry.example.com/team/app:2.0.0") {
		t.Fatal("expected different image references to produce different names")
	}
}

func TestReconcileCreatesImageAndRecordsObservation(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(imagev1.AddToScheme(scheme))

	pod := &corev1.Pod{}
	pod.Namespace = "default"
	pod.Name = "web"
	pod.Spec.Containers = []corev1.Container{{Name: "web", Image: "nginx:1.27"}}

	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod).
		WithStatusSubresource(&imagev1.Image{}).
		Build()

	reconciler := &PodReconciler{
		Client: client,
		Scheme: scheme,
		Now:    func() time.Time { return now },
		PlatformResolver: fakePlatformResolver{platforms: map[string]map[string]string{
			"nginx:1.27": {
				"linux/amd64": "nginx@sha256:amd64",
				"linux/arm64": "nginx@sha256:arm64",
			},
		}},
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "web"},
	})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var image imagev1.Image
	if err := client.Get(context.Background(), types.NamespacedName{Name: imageResourceName("nginx:1.27")}, &image); err != nil {
		t.Fatalf("expected Image to be created: %v", err)
	}

	if image.Spec.Reference != "nginx:1.27" {
		t.Fatalf("expected reference %q, got %q", "nginx:1.27", image.Spec.Reference)
	}
	if image.Spec.Platforms["linux/amd64"] != "nginx@sha256:amd64" {
		t.Fatalf("expected linux/amd64 platform digest, got %#v", image.Spec.Platforms)
	}
	if image.Spec.Platforms["linux/arm64"] != "nginx@sha256:arm64" {
		t.Fatalf("expected linux/arm64 platform digest, got %#v", image.Spec.Platforms)
	}
	if image.Status.LastSeen == nil || !image.Status.LastSeen.Time.Equal(now) {
		t.Fatalf("expected LastSeen %s, got %#v", now, image.Status.LastSeen)
	}
	if len(image.Status.ObservedIn) != 1 {
		t.Fatalf("expected one observation, got %#v", image.Status.ObservedIn)
	}
	if image.Status.ObservedIn[0].Namespace != "default" || image.Status.ObservedIn[0].Name != "web" {
		t.Fatalf("unexpected observation: %#v", image.Status.ObservedIn[0])
	}
}

func TestReconcileUpdatesExistingImagePlatforms(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(imagev1.AddToScheme(scheme))

	pod := &corev1.Pod{}
	pod.Namespace = "default"
	pod.Name = "web"
	pod.Spec.Containers = []corev1.Container{{Name: "web", Image: "nginx:1.27"}}

	image := &imagev1.Image{}
	image.Name = imageResourceName("nginx:1.27")
	image.Spec.Reference = "nginx:1.27"
	image.Spec.Platforms = map[string]string{"linux/amd64": "nginx@sha256:old"}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod, image).
		WithStatusSubresource(&imagev1.Image{}).
		Build()

	reconciler := &PodReconciler{
		Client: client,
		Scheme: scheme,
		Now:    func() time.Time { return time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC) },
		PlatformResolver: fakePlatformResolver{platforms: map[string]map[string]string{
			"nginx:1.27": {
				"linux/amd64": "nginx@sha256:new",
				"linux/arm64": "nginx@sha256:arm64",
			},
		}},
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "web"},
	})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var updated imagev1.Image
	if err := client.Get(context.Background(), types.NamespacedName{Name: image.Name}, &updated); err != nil {
		t.Fatalf("get image: %v", err)
	}
	if updated.Spec.Platforms["linux/amd64"] != "nginx@sha256:new" {
		t.Fatalf("expected updated linux/amd64 digest, got %#v", updated.Spec.Platforms)
	}
	if updated.Spec.Platforms["linux/arm64"] != "nginx@sha256:arm64" {
		t.Fatalf("expected added linux/arm64 digest, got %#v", updated.Spec.Platforms)
	}
}

func TestReconcileCreatesImageWhenPlatformResolutionFails(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(imagev1.AddToScheme(scheme))

	pod := &corev1.Pod{}
	pod.Namespace = "default"
	pod.Name = "web"
	pod.Spec.Containers = []corev1.Container{{Name: "web", Image: "private.example.com/app:v1"}}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod).
		WithStatusSubresource(&imagev1.Image{}).
		Build()

	reconciler := &PodReconciler{
		Client:           client,
		Scheme:           scheme,
		PlatformResolver: fakePlatformResolver{err: fmt.Errorf("unauthorized")},
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "web"},
	})
	if err == nil {
		t.Fatal("expected reconcile to report registry resolution error")
	}

	var image imagev1.Image
	if err := client.Get(context.Background(), types.NamespacedName{Name: imageResourceName("private.example.com/app:v1")}, &image); err != nil {
		t.Fatalf("expected Image to be created before resolution error: %v", err)
	}
	if image.Spec.Reference != "private.example.com/app:v1" {
		t.Fatalf("expected reference to be preserved, got %q", image.Spec.Reference)
	}
	if image.Spec.Platforms != nil {
		t.Fatalf("expected platforms to remain empty on resolution error, got %#v", image.Spec.Platforms)
	}
}
