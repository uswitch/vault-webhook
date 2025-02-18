package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/uswitch/vault-webhook/pkg/apis/vaultwebhook.uswitch.com/v1alpha1"
	v1 "k8s.io/api/core/v1"
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

	patch := addVault(&pod, "bah", databases)

	if len(patch) != 2 {
		t.Errorf("patch should have two items, got: %v", len(patch))
	}

	if patch[1].Op != "add" {
		t.Error("patch should be adding init containers")
	}

}

func makePodOwnedByKind(ownerKind string) *v1.Pod {
	return &v1.Pod{
		Spec: v1.PodSpec{Containers: []v1.Container{v1.Container{}}},
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				metav1.OwnerReference{
					Kind: ownerKind,
				},
			},
		},
	}
}

func vaultContainers(containers []v1.Container) []v1.Container {
	var vc []v1.Container
	for _, container := range containers {
		if strings.HasPrefix(container.Name, "vault-creds-") {
			vc = append(vc, container)
		}
	}

	return vc
}

func containersForPatch(patchOps []patchOperation) []v1.Container {
	for _, patch := range patchOps {
		if patch.Path == "/spec/containers" {
			return patch.Value.([]v1.Container)
		}
	}
	return []v1.Container{}
}

func checkJobFlagExists(container v1.Container) bool {
	for _, arg := range container.Args {
		if arg == "--job" {
			return true
		}
	}
	return false
}

func TestVaultJobMode(t *testing.T) {
	kindTestCases := map[string]bool{
		"Job":        true,
		"Workflow":   true,
		"Deployment": false,
		"FooBar":     false,
	}

	testNamespace := "testNamespace"
	testDatabases := []database{
		{database: "foo", role: "bar"},
	}

	for kind, shouldExist := range kindTestCases {
		t.Run(kind, func(t *testing.T) {
			pod := makePodOwnedByKind(kind)
			patchOps := addVault(pod, testNamespace, testDatabases)
			if len(patchOps) < 1 {
				t.Error("no patch operations returned from addVault function")
				return
			}

			containers := vaultContainers(containersForPatch(patchOps))
			if len(containers) != 1 {
				t.Errorf("incorrect number of vault sidecars in patch operation, expected=1, received=%d", len(containers))
			}

			for _, c := range containers {
				jobFlagExists := checkJobFlagExists(c)
				if jobFlagExists != shouldExist {
					t.Errorf("job flag state incorrect for king=%q, expected=%t, received=%t", kind, shouldExist, jobFlagExists)
				}
			}
		})
	}
}

// Can we add a preStop hook to the vault container?
func TestAddLifecyclePreStopHook(t *testing.T) {

	// Define test cases
	var tests = []struct {
		scenario     string
		lifecycleObj v1alpha1.Container
		answer       bool
	}{
		{
			scenario: "Test passing a complete lifecyle config - Exec Command",
			lifecycleObj: v1alpha1.Container{
				Lifecycle: v1.Lifecycle{
					PreStop: &v1.LifecycleHandler{
						Exec: &v1.ExecAction{
							Command: []string{"echo", "hello"},
						},
					},
				},
			},
			answer: true,
		},
		{
			scenario: "Test passing an incomplete lifecycle config",
			lifecycleObj: v1alpha1.Container{
				Lifecycle: v1.Lifecycle{
					PreStop: &v1.LifecycleHandler{
						Exec: nil,
					},
				},
			},
			answer: false,
		},
		{
			// v1alpha1.Container{}, comes from corev1.Container{} and this ALWAYS have a c.Lifecycle object. The latter, always has pointers to PostStart and PreStop handlers ( but no further down the struct since they are pointers )
			// if our dcb input does not specify a container object, the received input will look like this: {Lifecycle:{PostStart:nil PreStop:nil}}
			scenario: "Test passing no lifecycle config",
			lifecycleObj: v1alpha1.Container{
				Lifecycle: v1.Lifecycle{
					PreStop: nil,
				},
			},
			answer: false,
		},
		{
			scenario: "Test passing a complete lifecyle config - Sleep",
			lifecycleObj: v1alpha1.Container{
				Lifecycle: v1.Lifecycle{
					PreStop: &v1.LifecycleHandler{
						Sleep: &v1.SleepAction{
							Seconds: 10,
						},
					},
				},
			},
			answer: true,
		},
		{
			scenario: "Test passing an incorrect lifecyle config - Sleep",
			lifecycleObj: v1alpha1.Container{
				Lifecycle: v1.Lifecycle{
					PreStop: &v1.LifecycleHandler{
						Sleep: &v1.SleepAction{
							Seconds: -10,
						},
					},
				},
			},
			answer: false,
		},
	}

	// Run tests
	for _, tt := range tests {
		// t.Run enables running "subtests", one for each table entry. These are shown separately when executing `go test -v`.
		vaultContainer := v1.Container{} // Define a Vault sidecar Container
		testname := fmt.Sprintf("%v", tt.scenario)
		t.Run(testname, func(t *testing.T) {
			ans := addLifecycleHook(vaultContainer, tt.lifecycleObj)

			//log.Printf("%+v", ans)

			// Is our function returning a container object with valid Lifecycle config?
			isValid := bool(false)
			if ans.Lifecycle != nil && ans.Lifecycle.PreStop != nil {
				if ans.Lifecycle.PreStop.Exec != nil && len(ans.Lifecycle.PreStop.Exec.Command) > 0 { // Check preStop Exec format
					isValid = true
				} else if ans.Lifecycle.PreStop.Sleep != nil && ans.Lifecycle.PreStop.Sleep.Seconds > 0 { // Check preStop Sleep format
					isValid = true
				}
			}

			if isValid != tt.answer {
				t.Errorf("got %v, want %v", isValid, tt.answer)
			}
		})
	}

}
