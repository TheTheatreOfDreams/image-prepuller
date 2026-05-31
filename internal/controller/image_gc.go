package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	imagev1 "github.com/TheatreOfDreamsDev/prepuller/api/v1"
)

const DefaultImageTTL = 7 * 24 * time.Hour

type ImageGarbageCollector struct {
	client.Client
	TTL        time.Duration
	SyncPeriod time.Duration
	Now        func() time.Time
}

// +kubebuilder:rbac:groups=prepuller.theatreofdreams.dev,resources=images,verbs=get;list;watch;delete

func (g *ImageGarbageCollector) Start(ctx context.Context) error {
	if err := g.Sync(ctx); err != nil {
		return err
	}

	period := g.SyncPeriod
	if period <= 0 {
		period = time.Hour
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := g.Sync(ctx); err != nil {
				log.FromContext(ctx).Error(err, "image garbage collection failed")
			}
		}
	}
}

func (g *ImageGarbageCollector) Sync(ctx context.Context) error {
	ttl := g.TTL
	if ttl <= 0 {
		ttl = DefaultImageTTL
	}
	cutoff := metav1.NewTime(g.now().Add(-ttl))

	var images imagev1.ImageList
	if err := g.List(ctx, &images); err != nil {
		return fmt.Errorf("list images for garbage collection: %w", err)
	}

	for i := range images.Items {
		image := &images.Items[i]
		if !imageIsStale(image, cutoff) {
			continue
		}
		if err := g.Delete(ctx, image); err != nil {
			return fmt.Errorf("delete stale image %q: %w", image.Name, err)
		}
	}
	return nil
}

func (g *ImageGarbageCollector) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now().UTC()
}

func imageIsStale(image *imagev1.Image, cutoff metav1.Time) bool {
	return image.Status.LastSeen != nil && image.Status.LastSeen.Before(&cutoff)
}
