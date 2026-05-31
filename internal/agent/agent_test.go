package agent

import (
	"context"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	imagev1 "github.com/TheatreOfDreamsDev/prepuller/api/v1"
)

type fakeRuntime struct {
	images  map[string]struct{}
	pulled  []string
	removed []string
}

func (f *fakeRuntime) PullImage(_ context.Context, image string) error {
	f.pulled = append(f.pulled, image)
	f.images[image] = struct{}{}
	return nil
}

func (f *fakeRuntime) ListImages(context.Context) (map[string]struct{}, error) {
	images := map[string]struct{}{}
	for image := range f.images {
		images[image] = struct{}{}
	}
	return images, nil
}

func (f *fakeRuntime) RemoveImage(_ context.Context, image string) error {
	f.removed = append(f.removed, image)
	delete(f.images, image)
	return nil
}

func (f *fakeRuntime) Close() error {
	return nil
}

func TestNodeMatchesSelector(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		"node-role.kubernetes.io/worker": "true",
		"kubernetes.io/os":               "linux",
	}}}

	if !nodeMatchesSelector(node, "kubernetes.io/os=linux") {
		t.Fatal("expected node to match linux selector")
	}
	if nodeMatchesSelector(node, "kubernetes.io/os=windows") {
		t.Fatal("did not expect node to match windows selector")
	}
	if nodeMatchesSelector(node, "not a selector") {
		t.Fatal("did not expect invalid selector to match")
	}
}

func TestDesiredImagesForPlatform(t *testing.T) {
	images := []imagev1.Image{
		{Spec: imagev1.ImageSpec{Platforms: map[string]string{
			"linux/amd64": "nginx@sha256:amd64",
			"linux/arm64": "nginx@sha256:arm64",
		}}},
		{Spec: imagev1.ImageSpec{Platforms: map[string]string{
			"linux/arm64": "busybox@sha256:arm64",
		}}},
	}

	desired := desiredImagesForPlatform(images, "linux/arm64")
	assertSetContains(t, desired, "nginx@sha256:arm64")
	assertSetContains(t, desired, "busybox@sha256:arm64")
	if _, ok := desired["nginx@sha256:amd64"]; ok {
		t.Fatal("did not expect linux/amd64 image for linux/arm64 platform")
	}
}

func TestDesiredImagesForAllPlatforms(t *testing.T) {
	images := []imagev1.Image{
		{Spec: imagev1.ImageSpec{Platforms: map[string]string{
			"linux/amd64": "nginx@sha256:amd64",
			"linux/arm64": "nginx@sha256:arm64",
		}}},
		{Spec: imagev1.ImageSpec{Platforms: map[string]string{
			"linux/arm64": "busybox@sha256:arm64",
		}}},
	}

	desired := desiredImagesForAllPlatforms(images)
	assertSetContains(t, desired, "nginx@sha256:amd64")
	assertSetContains(t, desired, "nginx@sha256:arm64")
	assertSetContains(t, desired, "busybox@sha256:arm64")
}

func TestSyncPullsDesiredImagesAndRecordsState(t *testing.T) {
	scheme := testScheme()
	node := node("node-a", map[string]string{
		corev1.LabelOSStable:   "linux",
		corev1.LabelArchStable: "arm64",
		"prepuller":            "enabled",
	})
	image := &imagev1.Image{ObjectMeta: metav1.ObjectMeta{Name: "img-nginx"}}
	image.Spec.Platforms = map[string]string{"linux/arm64": "nginx@sha256:arm64"}

	store := NewStateStore(t.TempDir() + "/state.json")
	runtime := &fakeRuntime{images: map[string]struct{}{}}
	agent := &Agent{
		Client:  fakeClient(scheme, node, image),
		Runtime: runtime,
		Store:   store,
		Options: Options{NodeName: "node-a", NodeSelector: "prepuller=enabled"},
		Now:     func() time.Time { return time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC) },
	}

	if err := agent.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	assertSliceContains(t, runtime.pulled, "nginx@sha256:arm64")
	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if _, ok := state.Images["nginx@sha256:arm64"]; !ok {
		t.Fatal("expected pulled image to be recorded in state")
	}
}

func TestSyncSeedNodePullsAllPlatformImages(t *testing.T) {
	scheme := testScheme()
	node := node("node-a", map[string]string{
		corev1.LabelOSStable:                "linux",
		corev1.LabelArchStable:              "arm64",
		"prepuller.theatreofdreams.dev/seed": "true",
	})
	image := &imagev1.Image{ObjectMeta: metav1.ObjectMeta{Name: "img-nginx"}}
	image.Spec.Platforms = map[string]string{
		"linux/amd64": "nginx@sha256:amd64",
		"linux/arm64": "nginx@sha256:arm64",
	}

	store := NewStateStore(t.TempDir() + "/state.json")
	runtime := &fakeRuntime{images: map[string]struct{}{}}
	agent := &Agent{
		Client:  fakeClient(scheme, node, image),
		Runtime: runtime,
		Store:   store,
		Options: Options{
			NodeName:         "node-a",
			NodeSelector:     "kubernetes.io/os=linux",
			SeedNodeSelector: "prepuller.theatreofdreams.dev/seed=true",
		},
	}

	if err := agent.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	assertSliceContains(t, runtime.pulled, "nginx@sha256:amd64")
	assertSliceContains(t, runtime.pulled, "nginx@sha256:arm64")
}

func TestSyncGarbageCollectsOnlyPrepullerOwnedUnusedImages(t *testing.T) {
	scheme := testScheme()
	node := node("node-a", map[string]string{
		corev1.LabelOSStable:   "linux",
		corev1.LabelArchStable: "amd64",
	})
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}}
	pod.Spec.NodeName = "node-a"
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		ImageID: "docker-pullable://nginx@sha256:keep",
	}}

	store := NewStateStore(t.TempDir() + "/state.json")
	if err := store.Save(State{Images: map[string]ImageState{
		"nginx@sha256:keep":   {},
		"busybox@sha256:drop": {},
	}}); err != nil {
		t.Fatalf("save state: %v", err)
	}
	runtime := &fakeRuntime{images: map[string]struct{}{
		"nginx@sha256:keep":   {},
		"busybox@sha256:drop": {},
		"alpine@sha256:other": {},
	}}
	agent := &Agent{
		Client:  fakeClient(scheme, node, pod),
		Runtime: runtime,
		Store:   store,
		Options: Options{NodeName: "node-a", NodeSelector: "kubernetes.io/os=linux"},
	}

	if err := agent.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	assertSliceContains(t, runtime.removed, "busybox@sha256:drop")
	if slices.Contains(runtime.removed, "nginx@sha256:keep") {
		t.Fatal("did not expect running Pod image to be removed")
	}
	if slices.Contains(runtime.removed, "alpine@sha256:other") {
		t.Fatal("did not expect non-prepuller-owned image to be removed")
	}
}

func testScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(imagev1.AddToScheme(scheme))
	return scheme
}

func node(name string, labels map[string]string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
}

func fakeClient(scheme *runtime.Scheme, objects ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(object client.Object) []string {
			pod := object.(*corev1.Pod)
			if pod.Spec.NodeName == "" {
				return nil
			}
			return []string{pod.Spec.NodeName}
		}).
		Build()
}

func assertSetContains(t *testing.T, set map[string]struct{}, value string) {
	t.Helper()
	if _, ok := set[value]; !ok {
		t.Fatalf("expected set to contain %q", value)
	}
}

func assertSliceContains(t *testing.T, values []string, value string) {
	t.Helper()
	if !slices.Contains(values, value) {
		t.Fatalf("expected %v to contain %q", values, value)
	}
}
