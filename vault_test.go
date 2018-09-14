package main

import (
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddVolumeMount(t *testing.T) {
	containers := make([]v1.Container, 2)
	database := []database{
		database{
			outputPath: "/etc/foo",
		},
		database{
			outputPath: "/etc/bah",
		},
	}

	containers = addVolumeMount(containers, database)
	if len(containers) != 2 {
		t.Errorf("should be two containers, got :%v", len(containers))
	}

	if len(containers[0].VolumeMounts) != 2 || len(containers[1].VolumeMounts) != 2 {
		t.Errorf("should be two volumeMounts, got :%v,%v", len(containers[0].VolumeMounts), len(containers[1].VolumeMounts))
	}

	if containers[0].VolumeMounts[0].MountPath != "/etc/foo" || containers[0].VolumeMounts[1].MountPath != "/etc/bah" {
		t.Error("got unexpected volume mount paths")
	}

}

func TestAddSameVolumeMount(t *testing.T) {
	containers := make([]v1.Container, 1)
	database := []database{
		database{
			outputPath: "/etc/foo",
		},
		database{
			outputPath: "/etc/foo",
		},
	}
	containers = addVolumeMount(containers, database)

	if len(containers[0].VolumeMounts) != 1 {
		t.Error("got duplicate volume mounts")
	}
}

func TestAddVolume(t *testing.T) {
	pod := v1.Pod{}

	patch := addVolume(&pod)

	if patch[0].Path != "/spec/volumes" {
		t.Errorf("incorrect patch path: %v", patch[0].Path)
	}

	podWithVolume := v1.Pod{Spec: v1.PodSpec{
		Volumes: []v1.Volume{v1.Volume{}},
	}}

	patch = addVolume(&podWithVolume)
	if patch[0].Path != "/spec/volumes/-" {
		t.Errorf("incorrect patch path: %v", patch[0].Path)
	}
}

func TestAddVaultPatch(t *testing.T) {
	databases := []database{
		database{
			database: "foo",
			role:     "bah",
		},
		database{
			database: "baz",
			role:     "foo",
		},
	}

	pod := v1.Pod{
		Spec: v1.PodSpec{Containers: []v1.Container{v1.Container{}}},
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				metav1.OwnerReference{
					Kind: "Deployment",
				},
			},
		},
	}

	patch := addVault(&pod, "foo", "bah", databases)

	if len(patch) != 2 {
		t.Errorf("patch should have two items, got: %v", len(patch))
	}

	if patch[1].Op != "add" {
		t.Error("patch should be adding init containers")
	}

}
