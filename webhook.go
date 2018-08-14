package main

import (
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
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

type database struct {
	database   string
	role       string
	outputPath string
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

	log.Infof("AdmissionReview for Kind=%v, Namespace=%v Name=%v UID=%v patchOperation=%v UserInfo=%v",
		pod.ObjectMeta.OwnerReferences[0].Kind, req.Namespace, pod.ObjectMeta.OwnerReferences[0].Name, req.UID, req.Operation, req.UserInfo)

	binds, err := srv.bindings.List()
	if err != nil {
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	filteredBindings := filterBindings(binds, req.Namespace)
	if len(filteredBindings) == 0 {
		log.Infof("Skipping mutation for %s/%s, no database credential bindings in namespace", req.Namespace, pod.ObjectMeta.OwnerReferences[0].Name)
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	databases := matchBindings(filteredBindings, pod.Spec.ServiceAccountName)
	if len(databases) == 0 {
		log.Infof("Skipping mutation for %s/%s due to policy check", req.Namespace, pod.ObjectMeta.OwnerReferences[0].Name)
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	serviceAccountToken, err := srv.getServiceAccountToken(pod.Spec.ServiceAccountName, req.Namespace)
	if err != nil {
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}
	if serviceAccountToken == "" {
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: fmt.Errorf("no service account token was found for service account: %s", pod.Spec.ServiceAccountName).Error(),
			},
		}
	}

	patchBytes, err := createPatch(&pod, req.Namespace, serviceAccountToken, databases)
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

func (srv webHookServer) getServiceAccountToken(serviceAccount, namespace string) (string, error) {
	serviceAccountObj, err := srv.client.CoreV1().ServiceAccounts(namespace).Get(serviceAccount, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return serviceAccountObj.Secrets[0].Name, nil
}

func filterBindings(bindings []v1alpha1.DatabaseCredentialBinding, namespace string) []v1alpha1.DatabaseCredentialBinding {
	filteredBindings := []v1alpha1.DatabaseCredentialBinding{}
	for _, binding := range bindings {
		if binding.Namespace == namespace {
			filteredBindings = append(filteredBindings, binding)
		}
	}
	return filteredBindings
}

func matchBindings(bindings []v1alpha1.DatabaseCredentialBinding, serviceAccount string) []database {
	matchedBindings := []database{}
	for _, binding := range bindings {
		if binding.Spec.ServiceAccount == serviceAccount {
			output := binding.Spec.OutputPath
			if output == "" {
				output = "/etc/database"
			}
			matchedBindings = appendIfMissing(matchedBindings, database{role: binding.Spec.Role, database: binding.Spec.Database, outputPath: output})
		}
	}
	return matchedBindings
}

func appendIfMissing(slice []database, d database) []database {
	for _, ele := range slice {
		if ele == d {
			return slice
		}
	}
	return append(slice, d)
}
