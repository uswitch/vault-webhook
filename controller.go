package main

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/uswitch/vault-webhook/pkg/apis/vaultwebhook.uswitch.com/v1alpha1"
	webhookclient "github.com/uswitch/vault-webhook/pkg/client/clientset/versioned"
)

type bindingAggregator struct {
	store      cache.Store
	controller cache.Controller
}

func NewListWatch(client *webhookclient.Clientset) *bindingAggregator {
	binder := &bindingAggregator{}
	watcher := cache.NewListWatchFromClient(client.VaultwebhookV1alpha1().RESTClient(), "databasecredentialbindings", "", fields.Everything())

	informerOptions := cache.InformerOptions{
		ListerWatcher: watcher,
		ObjectType:    &v1alpha1.DatabaseCredentialBinding{},
		Handler:       binder,
		ResyncPeriod:  time.Minute,
		Indexers:      cache.Indexers{},
	}
	binder.store, binder.controller = cache.NewInformerWithOptions(informerOptions)
	cacheSize := prometheus.NewCounterFunc(
		prometheus.CounterOpts{
			Name: "database_credential_binding_cache_size",
			Help: "Current size of the Database Credential Binding cache",
		},
		func() float64 { return float64(binder.cacheSize()) },
	)
	prometheus.MustRegister(cacheSize)
	return binder
}

// https://pkg.go.dev/k8s.io/client-go/tools/cache#ResourceEventHandler
func (b *bindingAggregator) OnAdd(obj interface{}, isInInitialList bool) {
	log.Debugf("adding %+v", obj)
}

func (b *bindingAggregator) OnDelete(obj interface{}) {
	log.Debugf("deleting %+v", obj)
}

func (b *bindingAggregator) OnUpdate(old, new interface{}) {
	log.Debugf("updating %+v", new)
}

func (b *bindingAggregator) Run(ctx context.Context) error {
	go b.controller.Run(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), b.controller.HasSynced)
	log.Debugf("cache controller synced")

	return nil
}

func (b *bindingAggregator) List() ([]v1alpha1.DatabaseCredentialBinding, error) {
	bindingList := make([]v1alpha1.DatabaseCredentialBinding, 0)
	bindings := b.store.List()
	for _, obj := range bindings {
		binding, ok := obj.(*v1alpha1.DatabaseCredentialBinding)
		if !ok {
			return nil, fmt.Errorf("unexpected object in store: %+v", obj)
		}
		bindingList = append(bindingList, *binding)
	}
	return bindingList, nil

}

func (b *bindingAggregator) cacheSize() int {
	return len(b.store.List())
}
