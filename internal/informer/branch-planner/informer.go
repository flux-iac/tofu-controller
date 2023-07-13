package branchplanner

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"log"
	"strconv"
	"sync"
	"text/template"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/go-logr/logr"
	giturl "github.com/kubescape/go-git-url"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/internal/config"
	"github.com/weaveworks/tf-controller/internal/git/provider"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed comment.tpl
var commentTemplate string

type Informer struct {
	sharedInformer cache.SharedIndexInformer
	handlers       cache.ResourceEventHandlerFuncs
	log            logr.Logger
	client         client.Client
	gitProvider    provider.Provider

	mux    *sync.RWMutex
	synced bool
}

type Option func(s *Informer) error

func NewInformer(options ...Option) (*Informer, error) {
	informer := &Informer{}

	for _, opt := range options {
		if err := opt(informer); err != nil {
			return nil, err
		}
	}

	informer.mux = &sync.RWMutex{}
	informer.synced = false

	return informer, nil
}

func (i *Informer) HasSynced() bool {
	i.mux.RLock()
	defer i.mux.RUnlock()

	return i.synced
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

// updateHandler is called when a Terraform object is updated.
// It checks if the plan has been updated and if so, it creates a new PR comment to show the plan diff.
func (i *Informer) updateHandler(oldObj, newObj interface{}) {
	if !i.synced {
		return
	}
	i.mux.RLock()
	defer i.mux.RUnlock()

	old := &infrav1.Terraform{}
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

	new := &infrav1.Terraform{}
	newU, ok := newObj.(*unstructured.Unstructured)
	if !ok {
		i.log.Info("new object is not a unstructured.Unstructured object", "object", newObj)

		return
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(newU.Object, new); err != nil {
		i.log.Error(err, "failed to convert current object to Terraform object", "object", newObj)
		return
	}

	if new.Labels[config.LabelKey] != config.LabelValue {
		i.log.Info("Terraform object is not managed by the branch-based planner")

		return
	}

	if !i.isNewPlan(old, new) {
		i.log.Info("Plan not updated", "namespace", new.Namespace, "name", new.Name, "pr-id", new.Labels[config.LabelPRIDKey])

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

	i.log.Info("Updated plan", "pr-id", new.Labels[config.LabelPRIDKey])

	repo, err := i.getRepo(ctx, new)
	if err != nil {
		i.log.Error(err, "failed getting repository")
		return
	}

	number, err := strconv.Atoi(new.Labels[config.LabelPRIDKey])
	if err != nil {
		i.log.Error(err, "failed converting PR id to integer", "pr-id", new.Labels[config.LabelPRIDKey], "namespace", new.Namespace, "name", new.Name)
	}

	pr := provider.PullRequest{
		Repository: repo,
		Number:     number,
	}

	if _, err := i.gitProvider.AddCommentToPullRequest(ctx, pr, formatPlanOutput(planOutput)); err != nil {
		i.log.Error(err, "failed adding comment to pull request", "pr-id", new.Labels[config.LabelPRIDKey], "namespace", new.Namespace, "name", new.Name)
	}

}

func (i *Informer) deleteHandler(obj interface{}) {}

func (i *Informer) getPlan(ctx context.Context, obj *infrav1.Terraform) (*corev1.ConfigMap, error) {
	cmName := types.NamespacedName{Namespace: obj.GetNamespace(), Name: "tfplan-" + obj.WorkspaceName() + "-" + obj.GetName()}

	tfplanCM := &corev1.ConfigMap{}
	err := i.client.Get(ctx, cmName, tfplanCM)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &corev1.ConfigMap{
				Data: map[string]string{
					"tfplan": "Please set `spec.storeReadablePlan: human` to view the plan",
				},
			}, nil
		}

		return nil, fmt.Errorf("error getting plan cm: %s", err)
	}

	return tfplanCM, nil
}

func (i *Informer) isNewPlan(old, new *infrav1.Terraform) bool {
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

func (i *Informer) getRepo(ctx context.Context, tf *infrav1.Terraform) (provider.Repository, error) {
	if tf.Spec.SourceRef.Kind != sourcev1.GitRepositoryKind {
		return provider.Repository{}, fmt.Errorf("branch based planner does not support source kind: %s", tf.Spec.SourceRef.Kind)
	}

	ref := client.ObjectKey{
		Namespace: tf.Spec.SourceRef.Namespace,
		Name:      tf.Spec.SourceRef.Name,
	}
	obj := &sourcev1.GitRepository{}
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

func formatPlanOutput(planOutput string) []byte {
	type Output struct {
		PlanOutput string
	}

	data := Output{
		PlanOutput: planOutput,
	}

	tmpl, err := template.New("comment").Parse(commentTemplate)
	if err != nil {
		log.Fatalf("Error while parsing the template: %v", err)
	}

	var tpl bytes.Buffer
	if err := tmpl.Execute(&tpl, data); err != nil {
		log.Fatalf("Error while executing the template: %v", err)
	}

	return tpl.Bytes()
}
