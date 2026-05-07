package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ImageSpec describes a container image that has been observed in the cluster.
type ImageSpec struct {
	// Reference is the original image reference as it appeared on a Pod.
	Reference string `json:"reference"`
}

// ImageStatus records where and when an image was most recently observed.
type ImageStatus struct {
	// LastSeen is updated when the controller observes the image on a Pod.
	LastSeen *metav1.Time `json:"lastSeen,omitempty"`
	// ObservedIn lists Pods that referenced this image.
	ObservedIn []PodObservation `json:"observedIn,omitempty"`
}

// PodObservation identifies a Pod that referenced an image.
type PodObservation struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// Image is the Schema for the images API.
type Image struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageSpec   `json:"spec,omitempty"`
	Status ImageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ImageList contains a list of Images.
type ImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Image `json:"items"`
}
