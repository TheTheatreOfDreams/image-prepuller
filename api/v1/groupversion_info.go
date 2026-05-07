// Package v1 contains API Schema definitions for the image-prepuller v1 API group.
package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	Group   = "image-prepuller.theatreofdreams.io"
	Version = "v1alpha1"
)

var (
	GroupVersion  = schema.GroupVersion{Group: Group, Version: Version}
	SchemeBuilder = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(GroupVersion, &Image{}, &ImageList{})
		return nil
	})
	AddToScheme = SchemeBuilder.AddToScheme
)
