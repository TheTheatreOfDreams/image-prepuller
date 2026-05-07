package controller

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	imagev1 "github.com/TheTheatreOfDreams/prepuller/api/v1"
)

func TestImageGarbageCollectorDeletesStaleImages(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(imagev1.AddToScheme(scheme))

	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	staleTime := metav1.NewTime(now.Add(-8 * 24 * time.Hour))
	recentTime := metav1.NewTime(now.Add(-time.Hour))

	stale := &imagev1.Image{ObjectMeta: metav1.ObjectMeta{Name: "stale"}}
	stale.Status.LastSeen = &staleTime
	recent := &imagev1.Image{ObjectMeta: metav1.ObjectMeta{Name: "recent"}}
	recent.Status.LastSeen = &recentTime
	neverSeen := &imagev1.Image{ObjectMeta: metav1.ObjectMeta{Name: "never-seen"}}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(stale, recent, neverSeen).
		WithStatusSubresource(&imagev1.Image{}).
		Build()

	gc := &ImageGarbageCollector{
		Client: client,
		TTL:    DefaultImageTTL,
		Now:    func() time.Time { return now },
	}

	if err := gc.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	var image imagev1.Image
	if err := client.Get(context.Background(), types.NamespacedName{Name: "stale"}, &image); err == nil {
		t.Fatal("expected stale image to be deleted")
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "recent"}, &image); err != nil {
		t.Fatalf("expected recent image to remain: %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "never-seen"}, &image); err != nil {
		t.Fatalf("expected image without LastSeen to remain: %v", err)
	}
}
