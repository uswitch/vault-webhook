package v1alpha1

import (
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
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DatabaseCredentialBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []DatabaseCredentialBinding `json:"items"`
}

type Container struct {
	Lifecycle Lifecycle `json:"lifecycle,omitempty"`
}

type Lifecycle struct {
	PreStop LifecycleHandler `json:"preStop,omitempty"`
}

type LifecycleHandler struct {
	Exec ExecAction `json:"exec,omitempty"`
}

type ExecAction struct {
	Command []string `json:"command,omitempty"`
}
