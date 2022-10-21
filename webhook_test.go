package main

import (
	"testing"

	v1 "github.com/uswitch/vault-webhook/pkg/apis/vaultwebhook.uswitch.com/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFilterBindings(t *testing.T) {
	bindings := []v1.DatabaseCredentialBinding{
		v1.DatabaseCredentialBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
			},
		},
		v1.DatabaseCredentialBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bah",
			},
		},
	}

	bindings = filterBindings(bindings, "foo")

	if len(bindings) != 1 {
		t.Errorf("should have got one filtered binding, got: %v", len(bindings))
	}
}

func TestMatchBindings(t *testing.T) {

	bindings := []v1.DatabaseCredentialBinding{
		v1.DatabaseCredentialBinding{
			Spec: v1.DatabaseCredentialBindingSpec{
				ServiceAccount: "foo",
			},
		},
		v1.DatabaseCredentialBinding{
			Spec: v1.DatabaseCredentialBindingSpec{
				ServiceAccount: "bah",
			},
		},
	}

	databases := matchBindings(bindings, "bah")
	if len(databases) != 1 {
		t.Errorf("should have got one database, got: %v", len(databases))
	}
}
