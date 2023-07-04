package bbp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	tfv1alpha2 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/internal/git/provider"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Informer struct {
	sharedInformer cache.SharedIndexInformer
	handlers       cache.ResourceEventHandlerFuncs
	log            logr.Logger
	client         client.Client

	mux    *sync.RWMutex
	synced bool
}

func NewInformer(log logr.Logger, dynamicClient dynamic.Interface, clusterClient client.Client) (*Informer, error) {
	restMapper := clusterClient.RESTMapper()
	mapping, err := restMapper.RESTMapping(tfv1alpha2.GroupVersion.WithKind(tfv1alpha2.TerraformKind).GroupKind())
	if err != nil {
		log.Error(err, "failed to look up mapping for CRD")
		return nil, err
	}

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, time.Minute, corev1.NamespaceAll, nil)
	informer := factory.ForResource(mapping.Resource).Informer()

	return &Informer{
		sharedInformer: informer,
		mux:            &sync.RWMutex{},
		log:            log,
		synced:         false,
		client:         clusterClient,
	}, nil
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

const (
	LabelKey            = "infra.weave.works/branch-based-planner"
	LabelValue          = "true"
	LabelPRIDKey string = "infra.weave.works/pr-id"
)

func (i *Informer) addHandler(obj interface{}) {}

func (i *Informer) updateHandler(_, newObj interface{}) {
	if !i.synced {
		return
	}
	i.mux.RLock()
	defer i.mux.RUnlock()

	current, ok := newObj.(*tfv1alpha2.Terraform)
	if !ok {
		i.log.Info("current object is not a Terraform object", "object", newObj)

		return
	}

	if current.Labels[LabelKey] != LabelValue {
		i.log.Info("Terraform object is not managed by the branch-based planner")

		return
	}

	ctx := context.Background()

	plan, err := i.getPlan(ctx, current)
	if err != nil {
		i.log.Error(err, "get plan output")

		return
	}

	planOutput := plan.Data["tfplan"]
	if len(planOutput) == 0 {
		i.log.Info("Empty plan output")

		return
	}

	gitProvider, err := provider.New(provider.ProviderGitHub)
	if err != nil {
		i.log.Error(err, "unable to get provider", "provider", "github")

		return
	}

	gitProvider.AddCommentToPullRequest(ctx, provider.PullRequest{}, planOutput)
}

func (i *Informer) deleteHandler(obj interface{}) {}

func (i *Informer) getPlan(ctx context.Context, obj *tfv1alpha2.Terraform) (*corev1.Secret, error) {
	secretName := types.NamespacedName{Namespace: obj.GetNamespace(), Name: "tfplan-" + obj.WorkspaceName() + "-" + obj.GetName()}

	planSecret := &corev1.Secret{}
	err := i.client.Get(ctx, secretName, planSecret)
	if err != nil {
		err = fmt.Errorf("error getting plan secret: %s", err)

		return nil, err
	}

	return planSecret, nil
}
