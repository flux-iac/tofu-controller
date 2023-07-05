package bbp

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/go-logr/logr"
	giturl "github.com/kubescape/go-git-url"
	tfv1alpha2 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/internal/config"
	"github.com/weaveworks/tf-controller/internal/git/provider"
	"github.com/weaveworks/tf-controller/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Informer struct {
	sharedInformer cache.SharedIndexInformer
	handlers       cache.ResourceEventHandlerFuncs
	configMapRef   client.ObjectKey
	log            logr.Logger
	client         client.Client
	gitProvider    provider.Provider

	mux    *sync.RWMutex
	synced bool
}

type Option func(s *Informer) error

func NewInformer(dynamicClient dynamic.Interface, options ...Option) (*Informer, error) {
	informer := &Informer{}

	for _, opt := range options {
		if err := opt(informer); err != nil {
			return nil, err
		}
	}

	restMapper := informer.client.RESTMapper()
	mapping, err := restMapper.RESTMapping(tfv1alpha2.GroupVersion.WithKind(tfv1alpha2.TerraformKind).GroupKind())
	if err != nil {
		informer.log.Error(err, "failed to look up mapping for CRD")
		return nil, err
	}

	tweakListOptionsFunc := func(options *metav1.ListOptions) {
		options.LabelSelector = fmt.Sprintf("%s=%s", LabelKey, LabelValue)
	}

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, time.Minute, corev1.NamespaceAll, tweakListOptionsFunc)
	sharedInformer := factory.ForResource(mapping.Resource).Informer()

	informer.sharedInformer = sharedInformer
	informer.mux = &sync.RWMutex{}
	informer.synced = false

	ctx := context.Background()
	config, err := config.ReadConfig(ctx, informer.client, informer.configMapRef)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	secret, err := informer.getProviderSecret(ctx, client.ObjectKey{
		Namespace: config.SecretNamespace,
		Name:      config.SecretName,
	})
	if err != nil {
		informer.log.Error(err, "failed to get secret")
	}

	gitProvider, err := provider.New(provider.ProviderGitHub, provider.WithToken("api-token", string(secret.Data["token"])))
	if err != nil {
		return nil, fmt.Errorf("unable to get provider: %w", err)
	}

	informer.gitProvider = gitProvider

	return informer, nil
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

func (i *Informer) updateHandler(oldObj, newObj interface{}) {
	if !i.synced {
		return
	}
	i.mux.RLock()
	defer i.mux.RUnlock()

	old := &tfv1alpha2.Terraform{}
	oldU, ok := oldObj.(*unstructured.Unstructured)
	if !ok {
		i.log.Info("previous object is not a unstructured.Unstructured object", "object", oldObj)

		return
	}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(oldU.Object, old)
	if err != nil {
		i.log.Error(err, "failed to convert previous object to Terraform object", "object", oldObj)
		return
	}

	new := &tfv1alpha2.Terraform{}
	newU, ok := newObj.(*unstructured.Unstructured)
	if !ok {
		i.log.Info("new object is not a unstructured.Unstructured object", "object", newObj)

		return
	}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(newU.Object, new)
	if err != nil {
		i.log.Error(err, "failed to convert current object to Terraform object", "object", newObj)
		return
	}

	if new.Labels[LabelKey] != LabelValue {
		i.log.Info("Terraform object is not managed by the branch-based planner")

		return
	}

	if !i.isNewPlan(old, new) {
		i.log.Info("Plan not updated", "namespace", new.Namespace, "name", new.Name, "pr-id", new.Labels[LabelPRIDKey])

		return
	}

	ctx := context.Background()

	plan, err := i.getPlan(ctx, new)
	if err != nil {
		i.log.Error(err, "get plan output")

		return
	}

	planOutput := plan.Data["tfplan"]
	if len(planOutput) == 0 {
		i.log.Info("Empty plan output")

		return
	}

	fmt.Println("planOutput", string(planOutput))

	tfplan, err := utils.GzipDecode(planOutput)
	if err != nil {
		i.log.Error(err, "unable to decode the plan")
		return
	}

	fmt.Println("tfplan", string(tfplan))

	i.log.Info("Updated plan", "pr-id", new.Labels[LabelPRIDKey])

	repo, err := i.getRepo(ctx, new)
	if err != nil {
		i.log.Error(err, "failed getting repository")
		return
	}

	// convert a string to int
	number, err := strconv.Atoi(new.Labels[LabelPRIDKey])
	if err != nil {
		i.log.Error(err, "failed converting PR id to integer", "pr-id", new.Labels[LabelPRIDKey], "namespace", new.Namespace, "name", new.Name)
	}

	pr := provider.PullRequest{
		Repository: repo,
		Number:     number,
	}

	if _, err := i.gitProvider.AddCommentToPullRequest(ctx, pr, tfplan); err != nil {
		i.log.Error(err, "failed adding comment to pull request", "pr-id", new.Labels[LabelPRIDKey], "namespace", new.Namespace, "name", new.Name)
	}
}

func (i *Informer) getRepo(ctx context.Context, tf *tfv1alpha2.Terraform) (provider.Repository, error) {
	if tf.Spec.SourceRef.Kind != sourcev1b2.GitRepositoryKind {
		return provider.Repository{}, fmt.Errorf("branch based planner does not support source kind: %s", tf.Spec.SourceRef.Kind)
	}

	ref := client.ObjectKey{
		Namespace: tf.Spec.SourceRef.Namespace,
		Name:      tf.Spec.SourceRef.Name,
	}
	obj := &sourcev1b2.GitRepository{}
	if err := i.client.Get(ctx, ref, obj); err != nil {
		return provider.Repository{}, fmt.Errorf("unable to get Source: %w", err)
	}

	gitURL, err := giturl.NewGitURL(obj.Spec.URL)
	if err != nil {
		return provider.Repository{}, fmt.Errorf("failed parsing repository url: %w", err)
	}

	return provider.Repository{
		Org:  gitURL.GetOwnerName(),
		Name: gitURL.GetRepoName(),
	}, nil
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

func (i *Informer) isNewPlan(old, new *tfv1alpha2.Terraform) bool {
	if new.Status.LastPlanAt == nil {
		return false
	}

	if old.Status.LastPlanAt == nil && new.Status.LastPlanAt != nil {
		return true
	}

	if new.Status.LastPlanAt.After(old.Status.LastPlanAt.Time) {
		return true
	}

	return false
}

func (i *Informer) getProviderSecret(ctx context.Context, ref client.ObjectKey) (*corev1.Secret, error) {
	obj := &corev1.Secret{}
	if err := i.client.Get(ctx, ref, obj); err != nil {
		return nil, fmt.Errorf("unable to get Secret: %w", err)
	}

	return obj, nil
}
