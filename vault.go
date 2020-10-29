package main

import (
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func createPatch(pod *corev1.Pod, namespace, serviceAccountToken string, databases []database) ([]byte, error) {
	patch := []patchOperation{}
	patch = append(patch, addVolume(pod)...)
	pod.Spec.Containers = addVolumeMount(pod.Spec.Containers, databases)
	if len(pod.Spec.InitContainers) != 0 {
		pod.Spec.InitContainers = addVolumeMount(pod.Spec.InitContainers, databases)
	}
	patch = append(patch, addVault(pod, namespace, serviceAccountToken, databases)...)
	return json.Marshal(patch)
}

func addVault(pod *corev1.Pod, namespace, serviceAccountToken string, databases []database) (patch []patchOperation) {
	initContainers := []corev1.Container{}
	for _, databaseInfo := range databases {

		database := databaseInfo.database
		role := databaseInfo.role
		serviceAccount := pod.Spec.ServiceAccountName

		authRole := fmt.Sprintf("%s_%s_%s", database, namespace, serviceAccount)
		containerName := fmt.Sprintf("vault-creds-%s-%s", strings.Replace(database, "_", "-", -1), role)
		secretPath := fmt.Sprintf(secretPathFormat, database, role)
		templatePath := fmt.Sprintf("/creds/template/%s-%s", database, role)
		var outputPath string

		if databaseInfo.outputFile == "" {
			outputPath = fmt.Sprintf("/creds/output/%s-%s", database, role)
		} else {
			outputPath = fmt.Sprintf("/creds/output/%s", databaseInfo.outputFile)
		}

		requests := corev1.ResourceList{
			"cpu":    resource.MustParse("10m"),
			"memory": resource.MustParse("20Mi"),
		}

		limits := corev1.ResourceList{
			"cpu":    resource.MustParse("30m"),
			"memory": resource.MustParse("50Mi"),
		}

		vaultContainer := corev1.Container{
			Image:           sidecarImage,
			ImagePullPolicy: "Always",
			Resources: corev1.ResourceRequirements{
				Requests: requests,
				Limits:   limits,
			},
			Name: containerName,
			Args: []string{
				"--vault-addr=" + vaultAddr,
				"--gateway-addr=" + gatewayAddr,
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

		initContainer := vaultContainer

		if len(pod.ObjectMeta.OwnerReferences) != 0 {
			if pod.ObjectMeta.OwnerReferences[0].Kind == "Job" {
				vaultContainer.Args = append(vaultContainer.Args, "--job")
			}
		}

		pod.Spec.Containers = append(pod.Spec.Containers, vaultContainer)

		initContainer.Args = append(initContainer.Args, "--init")
		initContainer.Name = initContainer.Name + "-init"
		initContainers = append(initContainers, initContainer)
	}

	var initOp string
	if len(pod.Spec.InitContainers) != 0 {
		initContainers = append(initContainers, pod.Spec.InitContainers...)
		initOp = "replace"
	} else {
		initOp = "add"
	}

	patch = append(patch, []patchOperation{
		patchOperation{
			Op:    "replace",
			Path:  "/spec/containers",
			Value: pod.Spec.Containers,
		},
		patchOperation{
			Op:    initOp,
			Path:  "/spec/initContainers",
			Value: initContainers,
		}}...)

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

func addVolumeMount(containers []corev1.Container, databases []database) []corev1.Container {

	modifiedContainers := []corev1.Container{}

	for _, container := range containers {
		for _, database := range databases {
			volumeMount := corev1.VolumeMount{
				Name:      "vault-creds",
				MountPath: database.outputPath,
			}
			//we don't want to mount the same path twice
			container.VolumeMounts = appendVolumeMountIfMissing(container.VolumeMounts, volumeMount)
		}
		modifiedContainers = append(modifiedContainers, container)
	}

	return modifiedContainers
}

func appendVolumeMountIfMissing(slice []corev1.VolumeMount, v corev1.VolumeMount) []corev1.VolumeMount {
	for _, ele := range slice {
		if ele == v {
			return slice
		}
	}
	return append(slice, v)
}
