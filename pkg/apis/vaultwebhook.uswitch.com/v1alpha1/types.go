package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DatabaseCredentialBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              DatabaseCredentialBindingSpec `json:"spec"`
}

type DatabaseCredentialBindingSpec struct {
	Database       string    `json:"database"`
	Role           string    `json:"role"`
	OutputPath     string    `json:"outputPath"`
	OutputFile     string    `json:"outputFile"`
	ServiceAccount string    `json:"serviceAccount"`
	Container      Container `json:"container,omitempty"`
	// InitContainer  Container `json:"initcontainer,omitempty"` // TODO: Fix support for initcontainer's Lifecycle hooks  ( Go dep to be updated )
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DatabaseCredentialBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []DatabaseCredentialBinding `json:"items"`
}

type Container struct {
	Lifecycle corev1.Lifecycle `json:"lifecycle,omitempty"`
}
