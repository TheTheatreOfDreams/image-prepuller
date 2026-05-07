package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	imagev1 "github.com/TheTheatreOfDreams/prepuller/api/v1"
)

type PodReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Now              func() time.Time
	PlatformResolver ImagePlatformResolver
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=prepuller.theatreofdreams.io,resources=images,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=prepuller.theatreofdreams.io,resources=images/status,verbs=get;update;patch

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get pod: %w", err)
	}

	for _, reference := range imageReferencesFromPod(&pod) {
		if err := r.ensureImage(ctx, reference, nil); err != nil {
			return ctrl.Result{}, err
		}
		platforms, err := r.resolvePlatforms(ctx, reference)
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := r.ensureImage(ctx, reference, platforms); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.recordObservation(ctx, reference, pod.Namespace, pod.Name); err != nil {
			return ctrl.Result{}, err
		}
		logger.V(1).Info("observed image", "image", reference, "pod", req.NamespacedName)
	}

	return ctrl.Result{}, nil
}

func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}

func (r *PodReconciler) ensureImage(ctx context.Context, reference string, platforms map[string]string) error {
	name := imageResourceName(reference)
	var image imagev1.Image
	if err := r.Get(ctx, types.NamespacedName{Name: name}, &image); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get image %q: %w", name, err)
		}

		image = imagev1.Image{
			TypeMeta: metav1.TypeMeta{
				APIVersion: imagev1.GroupVersion.String(),
				Kind:       "Image",
			},
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: imagev1.ImageSpec{
				Reference: reference,
				Platforms: platforms,
			},
		}
		if err := r.Create(ctx, &image); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create image %q: %w", name, err)
		}
		return nil
	}

	if image.Spec.Reference == reference && (platforms == nil || maps.Equal(image.Spec.Platforms, platforms)) {
		return nil
	}

	image.Spec.Reference = reference
	if platforms != nil {
		image.Spec.Platforms = platforms
	}
	if err := r.Update(ctx, &image); err != nil {
		return fmt.Errorf("update image %q spec: %w", name, err)
	}
	return nil
}

func (r *PodReconciler) recordObservation(ctx context.Context, reference, namespace, podName string) error {
	name := imageResourceName(reference)
	now := metav1.NewTime(r.now())
	observation := imagev1.PodObservation{Namespace: namespace, Name: podName}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var image imagev1.Image
		if err := r.Get(ctx, types.NamespacedName{Name: name}, &image); err != nil {
			return fmt.Errorf("get image %q for status update: %w", name, err)
		}

		image.Status.LastSeen = &now
		if !hasObservation(image.Status.ObservedIn, observation) {
			image.Status.ObservedIn = append(image.Status.ObservedIn, observation)
			slices.SortFunc(image.Status.ObservedIn, func(a, b imagev1.PodObservation) int {
				return strings.Compare(a.Namespace+"/"+a.Name, b.Namespace+"/"+b.Name)
			})
		}

		if err := r.Status().Update(ctx, &image); err != nil {
			return fmt.Errorf("update image %q status: %w", name, err)
		}
		return nil
	})
}

func (r *PodReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now().UTC()
}

func (r *PodReconciler) resolvePlatforms(ctx context.Context, reference string) (map[string]string, error) {
	resolver := r.PlatformResolver
	if resolver == nil {
		resolver = RegistryResolverFromEnv()
	}

	platforms, err := resolver.ResolvePlatforms(ctx, reference)
	if err != nil {
		return nil, fmt.Errorf("resolve platforms for %q: %w", reference, err)
	}
	return platforms, nil
}

func imageReferencesFromPod(pod *corev1.Pod) []string {
	seen := map[string]struct{}{}
	for _, container := range pod.Spec.InitContainers {
		addImageReference(seen, container.Image)
	}
	for _, container := range pod.Spec.Containers {
		addImageReference(seen, container.Image)
	}
	for _, container := range pod.Spec.EphemeralContainers {
		addImageReference(seen, container.Image)
	}

	references := make([]string, 0, len(seen))
	for reference := range seen {
		references = append(references, reference)
	}
	slices.Sort(references)
	return references
}

func addImageReference(seen map[string]struct{}, reference string) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return
	}
	seen[reference] = struct{}{}
}

func imageResourceName(reference string) string {
	sum := sha256.Sum256([]byte(reference))
	return "img-" + hex.EncodeToString(sum[:])[:40]
}

func hasObservation(observations []imagev1.PodObservation, needle imagev1.PodObservation) bool {
	for _, observation := range observations {
		if observation == needle {
			return true
		}
	}
	return false
}
