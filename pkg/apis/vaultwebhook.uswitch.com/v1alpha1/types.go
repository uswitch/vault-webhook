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

/*
https://pkg.go.dev/k8s.io/api/core/v1#LifecycleHandler
Check if Container.Lifecycle.PreStop is valid. This is to avoid mishandling incomplete inputs like the below:

	{	"Lifecycle": {
			"PostStart": null,
			"PreStop": {
			  "Exec": null,  # <----- Missing Command!!
			  "HTTPGet": null,"TCPSocket": null, "Sleep": null}}}
*/
func (c Container) HasValidPreStop() bool {

	if c.Lifecycle.PreStop != nil {
		// is Exec set correctly?
		if c.Lifecycle.PreStop.Exec != nil && len(c.Lifecycle.PreStop.Exec.Command) > 0 {
			return true

			// is Sleep set correctly?
		} else if c.Lifecycle.PreStop.Sleep != nil && (c.Lifecycle.PreStop.Sleep.Seconds > 0) { // We do not like negative values here
			return true

			// TODO: Handle HTTPGet and TCPSocket usecases
		} else {
			return false
		}
	} else {
		return false
	}
}
