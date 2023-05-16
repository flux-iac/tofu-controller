package bbp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	tfv1alpha2 "github.com/weaveworks/tf-controller/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

type Informer struct {
	sharedInformer cache.SharedIndexInformer
	handlers       cache.ResourceEventHandlerFuncs
	log            logr.Logger

	mux    *sync.RWMutex
	synced bool
}

func NewInformer(clusterClient dynamic.Interface, log logr.Logger) Informer {
	resource := schema.GroupVersionResource{
		Group:    tfv1alpha2.GroupVersion.Group,
		Version:  tfv1alpha2.GroupVersion.Version,
		Resource: tfv1alpha2.TerraformKind,
	}
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(clusterClient, time.Minute, corev1.NamespaceAll, nil)
	informer := factory.ForResource(resource).Informer()

	return Informer{
		sharedInformer: informer,
		mux:            &sync.RWMutex{},
		log:            log,
		synced:         false,
	}
}

func (i *Informer) Start(ctx context.Context) error {
	if i.handlers.AddFunc == nil {
		i.handlers.AddFunc = i.addHandler
	}

	if i.handlers.UpdateFunc == nil {
		i.handlers.UpdateFunc = i.updateHandler
	}

	if i.handlers.DeleteFunc == nil {
		i.handlers.DeleteFunc = i.deleteHandler
	}

	i.sharedInformer.AddEventHandler(i.handlers)
	go i.sharedInformer.Run(ctx.Done())

	isSynced := cache.WaitForCacheSync(ctx.Done(), i.sharedInformer.HasSynced)
	i.mux.Lock()
	i.synced = isSynced
	i.mux.Unlock()

	if !i.synced {
		return fmt.Errorf("coudn't sync shared informer")
	}

	<-ctx.Done()

	return nil
}

func (i *Informer) SetAddHandler(fn func(interface{})) {
	i.handlers.AddFunc = fn
}

func (i *Informer) SetUpdateHandler(fn func(interface{}, interface{})) {
	i.handlers.UpdateFunc = fn
}

func (i *Informer) SetDeleteHandler(fn func(interface{})) {
	i.handlers.DeleteFunc = fn
}

func (i *Informer) addHandler(obj interface{}) {}

func (i *Informer) updateHandler(oldObj, newObj interface{}) {
}

func (i *Informer) deleteHandler(obj interface{}) {}
