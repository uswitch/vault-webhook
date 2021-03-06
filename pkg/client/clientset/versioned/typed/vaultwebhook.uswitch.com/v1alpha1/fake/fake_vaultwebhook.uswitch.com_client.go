// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "github.com/uswitch/vault-webhook/pkg/client/clientset/versioned/typed/vaultwebhook.uswitch.com/v1alpha1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeVaultwebhookV1alpha1 struct {
	*testing.Fake
}

func (c *FakeVaultwebhookV1alpha1) DatabaseCredentialBindings(namespace string) v1alpha1.DatabaseCredentialBindingInterface {
	return &FakeDatabaseCredentialBindings{c, namespace}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeVaultwebhookV1alpha1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
