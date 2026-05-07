package v1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestAddToSchemeRegistersMetaTypes(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme failed: %v", err)
	}

	kinds, _, err := scheme.ObjectKinds(&metav1.ListOptions{})
	if err != nil {
		t.Fatalf("expected ListOptions to be registered: %v", err)
	}

	for _, kind := range kinds {
		if kind.GroupVersion() == GroupVersion {
			return
		}
	}

	t.Fatalf("expected ListOptions to be registered for %s, got %v", GroupVersion, kinds)
}
