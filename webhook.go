package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/golang/glog"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
	server *http.Server
	client *kubernetes.Clientset
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func (srv webHookServer) serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		glog.Error("empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		glog.Errorf("Content-Type=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	var admissionResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		glog.Errorf("Can't decode body: %v", err)
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
		glog.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}
	glog.Infof("Ready to write reponse ...")
	if _, err := w.Write(resp); err != nil {
		glog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}

}

func (srv webHookServer) mutate(ar *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	req := ar.Request

	var typeMeta metav1.TypeMeta
	if err := json.Unmarshal(req.Object.Raw, &typeMeta); err != nil {
		glog.Errorf("Could not unmarshal raw object: %v", err)
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		glog.Errorf("Could not unmarshal raw object: %v", err)
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	glog.Infof("AdmissionReview for Kind=%v, Namespace=%v Name=%v UID=%v patchOperation=%v UserInfo=%v",
		pod.ObjectMeta.OwnerReferences[0].Kind, req.Namespace, pod.ObjectMeta.OwnerReferences[0].Name, req.UID, req.Operation, req.UserInfo)

	// determine whether to perform mutation
	if !mutationRequired(&pod.ObjectMeta) {
		glog.Infof("Skipping mutation for %s/%s due to policy check", req.Namespace, pod.ObjectMeta.OwnerReferences[0].Name)
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

	patchBytes, err := createPatch(&pod, req.Namespace, serviceAccountToken)
	if err != nil {
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	glog.Infof("AdmissionResponse: patch=%v\n", string(patchBytes))
	return &v1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *v1beta1.PatchType {
			pt := v1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

func mutationRequired(meta *metav1.ObjectMeta) bool {
	if meta.GetAnnotations()["vault.uswitch.com/database"] != "" {
		return true
	}
	return false
}

func createPatch(pod *corev1.Pod, namespace, serviceAccountToken string) ([]byte, error) {
	patch := []patchOperation{}
	patch = append(patch, addVolume(pod)...)
	patch = append(patch, addVault(pod, namespace, serviceAccountToken)...)
	return json.Marshal(patch)
}

func addVault(pod *corev1.Pod, namespace, serviceAccountToken string) (patch []patchOperation) {

	database := pod.ObjectMeta.GetAnnotations()["vault.uswitch.com/database"]
	role := pod.ObjectMeta.GetAnnotations()["vault.uswitch.com/role"]
	serviceAccount := pod.Spec.ServiceAccountName

	authRole := fmt.Sprintf("%s_%s_%s", database, namespace, serviceAccount)
	containerName := fmt.Sprintf("vault-creds-%s-%s", strings.Replace(database, "_", "-", -1), role)
	secretPath := fmt.Sprintf("database/creds/%s_%s", database, role)
	templatePath := fmt.Sprintf("/creds/template/%s-%s", database, role)
	outputPath := fmt.Sprintf("/creds/output/%s-%s", database, role)

	vaultAddr := fmt.Sprintf("https://vault.%s.kube.usw.co", cluster)
	loginPath := fmt.Sprintf("kubernetes/%s/login", cluster)

	requests := corev1.ResourceList{
		"cpu":    resource.MustParse("10m"),
		"memory": resource.MustParse("20Mi"),
	}

	limits := corev1.ResourceList{
		"cpu":    resource.MustParse("30m"),
		"memory": resource.MustParse("50Mi"),
	}

	vaultContainer := corev1.Container{
		Image:           "registry.usw.co/cloud/vault-creds",
		ImagePullPolicy: "Always",
		Resources: corev1.ResourceRequirements{
			Requests: requests,
			Limits:   limits,
		},
		Name: containerName,
		Args: []string{
			"--vault-addr=" + vaultAddr,
			"--ca-cert=/vault.ca",
			"--secret-path=" + secretPath,
			"--login-path=" + loginPath,
			"--auth-role=" + authRole,
			"--template=" + templatePath,
			"--out=" + outputPath,
			"--completed-path=/creds/output/completed",
			"--renew-interval=1h",
			"--lease-duration=12h",
			"--json-log",
		},
		Env: []corev1.EnvVar{
			corev1.EnvVar{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			corev1.EnvVar{
				Name: "NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			corev1.VolumeMount{
				Name:      "vault-template",
				MountPath: "/creds/template",
			},
			corev1.VolumeMount{
				Name:      "vault-creds",
				MountPath: "/creds/output",
			},
			corev1.VolumeMount{
				Name:      serviceAccountToken,
				MountPath: "/var/run/secrets/kubernetes.io/serviceaccount",
			},
		},
	}

	patch = append(patch, patchOperation{
		Op:    "add",
		Path:  "/spec/containers/-",
		Value: vaultContainer,
	})

	initContainer := vaultContainer
	initContainer.Args = append(initContainer.Args, "--init")
	initContainer.Name = initContainer.Name + "-init"
	var init interface{}

	initPath := "/spec/initContainers"
	if len(pod.Spec.InitContainers) != 0 {
		initPath = initPath + "/-"
		init = initContainer
	} else {
		init = []corev1.Container{initContainer}
	}

	patch = append(patch, patchOperation{
		Op:    "add",
		Path:  initPath,
		Value: init,
	})

	return patch
}

func addVolume(pod *corev1.Pod) (patch []patchOperation) {

	volume := corev1.Volume{
		Name: "vault-creds",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	path := "/spec/volumes"
	var value interface{}

	if len(pod.Spec.Volumes) != 0 {
		path = path + "/-"
		value = volume
	} else {
		value = []corev1.Volume{volume}
	}

	patch = append(patch, patchOperation{
		Op:    "add",
		Path:  path,
		Value: value,
	})

	return patch
}

func (srv webHookServer) getServiceAccountToken(serviceAccount, namespace string) (string, error) {
	serviceAccountObj, err := srv.client.CoreV1().ServiceAccounts(namespace).Get(serviceAccount, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return serviceAccountObj.Secrets[0].Name, nil
}
