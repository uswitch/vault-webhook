package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	log "github.com/sirupsen/logrus"
	"github.com/uswitch/vault-webhook/pkg/apis/vaultwebhook.uswitch.com/v1alpha1"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"k8s.io/client-go/kubernetes"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

type webHookServer struct {
	server   *http.Server
	client   *kubernetes.Clientset
	bindings *bindingAggregator
	ctx      context.Context
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

type database struct {
	database       string
	role           string
	outputPath     string
	outputFile     string
	vaultContainer v1alpha1.Container
	// initVaultContainer v1alpha1.Container // TODO: Fix support for initcontainer's Lifecycle hooks  ( Go dep to be updated )
}

func (srv webHookServer) serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		log.Error("empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Errorf("Content-Type=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	var admissionResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		log.Errorf("Can't decode body: %v", err)
		admissionResponse = &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		admissionResponse = srv.mutate(&ar)
	}

	admissionReview := v1beta1.AdmissionReview{}
	if admissionResponse != nil {
		admissionReview.Response = admissionResponse
		if ar.Request != nil {
			admissionReview.Response.UID = ar.Request.UID
		}
	}

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		log.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}
	log.Infof("Ready to write reponse ...")
	if _, err := w.Write(resp); err != nil {
		log.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}

}

// This handles the admission review sent by k8s and mutates the pod
func (srv webHookServer) mutate(ar *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	req := ar.Request

	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		log.Errorf("Could not unmarshal raw object: %v", err)
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	var ownerKind, ownerName string

	if len(pod.ObjectMeta.OwnerReferences) != 0 {
		ownerKind = pod.ObjectMeta.OwnerReferences[0].Kind
		ownerName = pod.ObjectMeta.OwnerReferences[0].Name
	}
	log.Infof("AdmissionReview for Kind=%v, Namespace=%v Name=%v UID=%v patchOperation=%v UserInfo=%v",
		ownerKind, req.Namespace, ownerName, req.UID, req.Operation, req.UserInfo)

	// A list of ALL the bindings.
	binds, err := srv.bindings.List()
	log.Infof("[mutate] List of all bindings: %+v", binds)
	if err != nil {
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	// Filter out the bindings that are not in the target namespace
	filteredBindings := filterBindings(binds, req.Namespace)
	if len(filteredBindings) == 0 {
		log.Infof("Skipping mutation for %s/%s, no database credential bindings in namespace", req.Namespace, ownerName)
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	// Identify bindings with ServiceAccount field matching the pod's ServiceAccountName
	databases := matchBindings(filteredBindings, pod.Spec.ServiceAccountName)
	if len(databases) == 0 {
		log.Infof("Skipping mutation for %s/%s due to policy check", req.Namespace, ownerName)
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	patchBytes, err := createPatch(&pod, req.Namespace, databases)
	if err != nil {
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	log.Infof("AdmissionResponse: patch=%v\n", string(patchBytes))
	return &v1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *v1beta1.PatchType {
			pt := v1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

// For all the bindings, we need to find the ones in the target namespace
func filterBindings(bindings []v1alpha1.DatabaseCredentialBinding, namespace string) []v1alpha1.DatabaseCredentialBinding {
	filteredBindings := []v1alpha1.DatabaseCredentialBinding{}
	for _, binding := range bindings {
		if binding.Namespace == namespace {
			filteredBindings = append(filteredBindings, binding)
		}
	}
	return filteredBindings
}

/*
	    For all the bindings in the namespace, check which one has a ServiceeAccount that matches the pod's ServiceAccount
		  - We could have multiple database specifications to be attached to a single pod.
		  - This means that we could also have different VaultContainer specs for each DatabaseCredentialBinding.
		  - As a consequence, to keep things consistent and easy to follow, we are appending into the `database` slice.
*/
func matchBindings(bindings []v1alpha1.DatabaseCredentialBinding, serviceAccount string) []database {
	matchedBindings := []database{}
	for _, binding := range bindings {
		if binding.Spec.ServiceAccount == serviceAccount {
			output := binding.Spec.OutputPath
			if output == "" {
				output = "/etc/database"
			}
			log.Infof("[matchBindings] Printing content of Container: %+v", binding.Spec.Container)
			//log.Infof("[matchBindings] Printing content of InitContainer: %+v", binding.Spec.InitContainer)

			matchedBindings = appendIfMissing(matchedBindings, database{
				role:           binding.Spec.Role,
				database:       binding.Spec.Database,
				outputPath:     output,
				outputFile:     binding.Spec.OutputFile,
				vaultContainer: binding.Spec.Container,
				//initVaultContainer: binding.Spec.InitContainer, // TODO: Fix support for initcontainer's Lifecycle hooks  ( Go dep to be updated )
			})
		}
	}
	return matchedBindings
}

func appendIfMissing(slice []database, d database) []database {
	for _, ele := range slice {
		// No need to compare the Container and InitContainer fields.
		if ele.role == d.role &&
			ele.database == d.database &&
			ele.outputPath == d.outputPath &&
			ele.outputFile == d.outputFile {
			return slice
		}
	}
	return append(slice, d)
}

func (srv webHookServer) checkHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
}
