package agent

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	imagev1 "github.com/TheTheatreOfDreams/prepuller/api/v1"
)

type Options struct {
	NodeName         string
	NodeSelector     string
	SeedNodeSelector string
	RuntimeEndpoint  string
	StateFile        string
	SyncPeriod       time.Duration
}

type Agent struct {
	Client  client.Client
	Runtime Runtime
	Store   *StateStore
	Options Options
	Now     func() time.Time
}

func (a *Agent) Run(ctx context.Context) error {
	logger := log.FromContext(ctx).WithValues("node", a.Options.NodeName)
	logger.Info("starting prepuller agent", "syncPeriod", a.Options.SyncPeriod, "nodeSelector", a.Options.NodeSelector, "seedNodeSelector", a.Options.SeedNodeSelector)

	if err := a.Sync(ctx); err != nil {
		return err
	}

	period := a.Options.SyncPeriod
	if period <= 0 {
		period = time.Minute
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := a.Sync(ctx); err != nil {
				log.FromContext(ctx).Error(err, "sync failed")
			}
		}
	}
}

func (a *Agent) Sync(ctx context.Context) error {
	if a.Options.NodeName == "" {
		return fmt.Errorf("node name is required")
	}
	logger := log.FromContext(ctx).WithValues("node", a.Options.NodeName)

	var node corev1.Node
	if err := a.Client.Get(ctx, client.ObjectKey{Name: a.Options.NodeName}, &node); err != nil {
		return fmt.Errorf("get node %q: %w", a.Options.NodeName, err)
	}

	state, err := a.Store.Load()
	if err != nil {
		return err
	}

	desired := map[string]struct{}{}
	mode := "disabled"
	platform := nodePlatform(&node)
	if nodeMatchesNonEmptySelector(&node, a.Options.SeedNodeSelector) {
		mode = "seed"
		desired, err = a.desiredAllImages(ctx)
		if err != nil {
			return err
		}
	} else if nodeMatchesSelector(&node, a.Options.NodeSelector) {
		mode = "normal"
		desired, err = a.desiredImages(ctx, platform)
		if err != nil {
			return err
		}
	}

	runtimeImages, err := a.Runtime.ListImages(ctx)
	if err != nil {
		return err
	}

	now := a.now()
	pulled := 0
	for image := range desired {
		if _, ok := runtimeImages[image]; !ok {
			logger.Info("pulling desired image", "image", image, "mode", mode)
			if err := a.Runtime.PullImage(ctx, image); err != nil {
				return err
			}
			pulled++
		}
		state.Images[image] = ImageState{PulledAt: now}
	}

	pods, err := a.podsOnNode(ctx)
	if err != nil {
		return err
	}
	inUse := imagesUsedByPods(pods)
	removed := 0
	for image := range state.Images {
		if _, stillDesired := desired[image]; stillDesired {
			continue
		}
		if imageInUse(image, inUse) {
			continue
		}
		logger.Info("removing stale prepuller-owned image", "image", image)
		if err := a.Runtime.RemoveImage(ctx, image); err != nil {
			return err
		}
		delete(state.Images, image)
		removed++
	}

	logger.Info(
		"sync complete",
		"mode", mode,
		"platform", platform,
		"desiredImages", len(desired),
		"runtimeImages", len(runtimeImages),
		"trackedImages", len(state.Images),
		"podsOnNode", len(pods),
		"pulledImages", pulled,
		"removedImages", removed,
	)

	return a.Store.Save(state)
}

func (a *Agent) desiredImages(ctx context.Context, platform string) (map[string]struct{}, error) {
	var images imagev1.ImageList
	if err := a.Client.List(ctx, &images); err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	return desiredImagesForPlatform(images.Items, platform), nil
}

func (a *Agent) desiredAllImages(ctx context.Context) (map[string]struct{}, error) {
	var images imagev1.ImageList
	if err := a.Client.List(ctx, &images); err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	return desiredImagesForAllPlatforms(images.Items), nil
}

func (a *Agent) podsOnNode(ctx context.Context) ([]corev1.Pod, error) {
	var pods corev1.PodList
	if err := a.Client.List(ctx, &pods, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", a.Options.NodeName),
	}); err != nil {
		return nil, fmt.Errorf("list pods on node %q: %w", a.Options.NodeName, err)
	}
	return pods.Items, nil
}

func (a *Agent) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now().UTC()
}

func desiredImagesForPlatform(images []imagev1.Image, platform string) map[string]struct{} {
	desired := map[string]struct{}{}
	for _, image := range images {
		if image.Spec.Platforms == nil {
			continue
		}
		if digest, ok := image.Spec.Platforms[platform]; ok && digest != "" {
			desired[digest] = struct{}{}
		}
	}
	return desired
}

func desiredImagesForAllPlatforms(images []imagev1.Image) map[string]struct{} {
	desired := map[string]struct{}{}
	for _, image := range images {
		for _, digest := range image.Spec.Platforms {
			if digest != "" {
				desired[digest] = struct{}{}
			}
		}
	}
	return desired
}

func nodeMatchesSelector(node *corev1.Node, selector string) bool {
	if selector == "" {
		return true
	}
	parsed, err := labels.Parse(selector)
	if err != nil {
		return false
	}
	return parsed.Matches(labels.Set(node.Labels))
}

func nodeMatchesNonEmptySelector(node *corev1.Node, selector string) bool {
	return selector != "" && nodeMatchesSelector(node, selector)
}

func nodePlatform(node *corev1.Node) string {
	os := node.Labels[corev1.LabelOSStable]
	arch := node.Labels[corev1.LabelArchStable]
	if os == "" {
		os = runtime.GOOS
	}
	if arch == "" {
		arch = runtime.GOARCH
	}
	return os + "/" + arch
}

func imagesUsedByPods(pods []corev1.Pod) map[string]struct{} {
	images := map[string]struct{}{}
	for _, pod := range pods {
		addContainerImages(images, pod.Spec.InitContainers)
		addContainerImages(images, pod.Spec.Containers)
		for _, container := range pod.Spec.EphemeralContainers {
			if container.Image != "" {
				images[container.Image] = struct{}{}
			}
		}
		addContainerStatusImages(images, pod.Status.InitContainerStatuses)
		addContainerStatusImages(images, pod.Status.ContainerStatuses)
		addContainerStatusImages(images, pod.Status.EphemeralContainerStatuses)
	}
	return images
}

func addContainerImages(images map[string]struct{}, containers []corev1.Container) {
	for _, container := range containers {
		addImageReference(images, container.Image)
	}
}

func addContainerStatusImages(images map[string]struct{}, statuses []corev1.ContainerStatus) {
	for _, status := range statuses {
		addImageReference(images, status.Image)
		addImageReference(images, status.ImageID)
	}
}

func addImageReference(images map[string]struct{}, image string) {
	if image == "" {
		return
	}
	images[image] = struct{}{}
	for _, prefix := range []string{"docker-pullable://", "docker://", "containerd://"} {
		if unprefixed := strings.TrimPrefix(image, prefix); unprefixed != image {
			images[unprefixed] = struct{}{}
		}
	}
}

func imageInUse(image string, inUse map[string]struct{}) bool {
	if _, ok := inUse[image]; ok {
		return true
	}
	_, digest, ok := strings.Cut(image, "@")
	if !ok || digest == "" {
		return false
	}
	for used := range inUse {
		if strings.Contains(used, digest) {
			return true
		}
	}
	return false
}
