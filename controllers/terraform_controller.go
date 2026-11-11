/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	runtimeCtrl "github.com/fluxcd/pkg/runtime/controller"
	"github.com/fluxcd/pkg/runtime/dependency"
	"github.com/fluxcd/pkg/runtime/patch"
	"github.com/fluxcd/pkg/runtime/predicates"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	kuberecorder "k8s.io/client-go/tools/events"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/mtls"
	"github.com/flux-iac/tofu-controller/runner"
)

// TerraformReconciler reconciles a Terraform object
type TerraformReconciler struct {
	client.Client
	kuberecorder.EventRecorder
	runtimeCtrl.Metrics

	httpClient        *retryablehttp.Client
	FieldManager      string
	patchOptions      []patch.Option
	requeueDependency time.Duration

	// EnableExternalArtifact allows ExternalArtifact as a sourceRef kind. The kind only exists
	// with Flux 2.7+, so watching it on an older cluster stops the controller from starting.
	EnableExternalArtifact bool

	// Quota retry configuration
	QuotaRetryEnabled      bool
	QuotaRetryDelay        time.Duration
	QuotaRetryJitterFactor float64

	StatusPoller              *polling.StatusPoller
	Scheme                    *runtime.Scheme
	CertRotator               *mtls.CertRotator
	RunnerGRPCPort            int
	RunnerCreationTimeout     time.Duration
	RunnerRPCTimeout          time.Duration
	RunnerUnreachableTimeout  time.Duration
	RunnerGRPCMaxMessageSize  int
	AllowBreakTheGlass        bool
	ClusterDomain             string
	NoCrossNamespaceRefs      bool
	UsePodSubdomainResolution bool
	Clientset                 *kubernetes.Clientset

	// Graceful shutdown fields
	ShutdownTimeout       time.Duration
	shutdownStarted       atomic.Bool
	activeReconciliations sync.WaitGroup

	// observeReconcile is a test-only hook, nil in production. It is called once
	// per Reconcile call that fetched the object, after the deferred status patch.
	observeReconcile func(req ctrl.Request, obj *infrav1.Terraform, res ctrl.Result, err error, panicked bool)

	// lookupOrCreateRunner is a test-only hook, nil in production. When set,
	// openSession calls it instead of LookupOrCreateRunner.
	lookupOrCreateRunner func(ctx context.Context, obj *infrav1.Terraform, revision string) (runner.RunnerClient, func() error, error)
}

//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms/finalizers,verbs=get;create;update;patch;delete
//+kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=buckets;externalartifacts;gitrepositories;ocirepositories,verbs=get;list;watch
//+kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=buckets/status;externalartifacts/status;gitrepositories/status;ocirepositories/status,verbs=get
//+kubebuilder:rbac:groups="",resources=configmaps;secrets;serviceaccounts,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// It checks the prerequisites (source, schedule, dependencies, approval,
// retries) on the fetched object, then hands a copy to reconcileNormal, which
// talks to the runner. Deletion is handed to reconcileDelete. Whatever path a
// pass takes, a deferred patch writes the final status.
func (r *TerraformReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	start := time.Now()
	reconcileID := uuid.New().String()
	log := ctrl.LoggerFrom(ctx, "reconciliation-loop-id", reconcileID, "start-time", start)
	ctx = ctrl.LoggerInto(ctx, log)

	// Take no new work once shutdown has begun.
	if r.shutdownStarted.Load() {
		return ctrl.Result{}, nil
	}

	// Let a graceful shutdown wait for this pass to finish.
	r.activeReconciliations.Add(1)
	defer r.activeReconciliations.Done()

	// The runner connection needs valid mTLS certificates.
	r.waitForCertificates()

	obj := &infrav1.Terraform{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Deferred before the status patch below, so it runs after it and sees
	// the patched object.
	defer r.observeOutcome(req, &obj, &result, &retErr)

	log.Info(">> Started", "generation", obj.GetGeneration())

	// Every path ends by patching status. The deferred call reads obj when it
	// runs, so it patches the object the pipeline returned.
	patcher := patch.NewSerialPatcher(obj, r.Client)
	defer func() {
		retErr = r.patchFinalStatus(ctx, patcher, obj, start, retErr)
	}()

	// Deletion is handled before the finalizer and suspend checks, so a
	// suspended object can still be deleted.
	if isBeingDeleted(obj) {
		return r.reconcileDelete(ctx, patcher, obj, reconcileID)
	}

	// Add our finalizer and come back with it in place.
	if !controllerutil.ContainsFinalizer(obj, infrav1.TerraformFinalizer) {
		controllerutil.AddFinalizer(obj, infrav1.TerraformFinalizer)
		return ctrl.Result{Requeue: true}, nil
	}

	if obj.Spec.Suspend {
		log.Info("Reconciliation is suspended for this object")
		return ctrl.Result{}, nil
	}

	// Show that this pass is checking prerequisites.
	if err := r.markReconciling(ctx, patcher, obj); err != nil {
		return ctrl.Result{}, err
	}

	// Stop unless the source exists and has an artifact.
	src, ok, res, err := r.resolveArtifact(ctx, obj)
	if !ok {
		return res, err
	}
	revision := src.GetArtifact().Revision

	// Skip when nothing changed and the interval hasn't elapsed.
	if ok, reason, requeueAfter := r.shouldReconcile(obj, src); !ok {
		return skipReconcile(ctx, obj, reason, requeueAfter)
	}

	// Wait until every dependsOn target is ready.
	if len(obj.Spec.DependsOn) > 0 {
		if err := r.checkDependencies(ctx, obj, src); err != nil {
			return r.dependenciesNotReady(ctx, patcher, obj, revision, err)
		}
		log.Info("All dependencies are ready, proceeding with reconciliation")
	}

	// The prerequisites hold: show progress, reset the retry count when the
	// source or spec changed, and drop a pending plan that is now stale.
	clearPrerequisiteFailure(obj)
	if err := r.markProgressing(ctx, patcher, obj); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.resetRetryAndStalePlan(ctx, patcher, obj, revision); err != nil {
		return ctrl.Result{}, err
	}

	// Stop while a pending plan waits for manual approval.
	if r.awaitingApproval(obj) {
		log.Info("reconciliation is stopped to wait for a manual approve")
		return ctrl.Result{}, nil
	}

	// Stop when the retries are exhausted. This is checked before the runner
	// pod is created, so a stalled object does not start one.
	if !obj.ShouldRetry() {
		return r.retryLimitReached(ctx, patcher, obj)
	}

	// Start or reuse the runner pod and connect to it. Closing the session
	// cleans up the runner.
	sess, err := r.openSession(ctx, patcher, obj, revision, reconcileID)
	if err != nil {
		return r.runnerError(ctx, obj, err)
	}
	defer sess.Close(ctx)

	// Run the pipeline on a copy, which replaces obj only once it returns. If
	// the pipeline panics, the final status patch still sees the object as it
	// was before the pipeline ran.
	pass := obj.DeepCopy()
	reconcileErr := r.reconcileNormal(ctx, sess, pass, src)
	obj = pass

	// Record the outcome and choose when to come back.
	return r.result(ctx, patcher, obj, src, reconcileErr, start)
}

// observeOutcome reports the pass to the observeReconcile test hook. It is
// deferred, so it can recover a panic, report it, and panic again. obj is
// passed by address because Reconcile replaces it with the pipeline's copy.
// In production the hook is nil and a panic is left untouched.
func (r *TerraformReconciler) observeOutcome(req ctrl.Request, obj **infrav1.Terraform, result *ctrl.Result, retErr *error) {
	if r.observeReconcile == nil {
		return
	}
	if p := recover(); p != nil {
		r.observeReconcile(req, *obj, *result, *retErr, true)
		panic(p)
	}
	r.observeReconcile(req, *obj, *result, *retErr, false)
}

// waitForCertificates blocks until the cert rotator is ready. When the CA is
// invalid it also triggers a CA rotation and waits for it.
func (r *TerraformReconciler) waitForCertificates() {
	<-r.CertRotator.Ready

	if isCAValid, _ := r.CertRotator.IsCAValid(); !isCAValid && r.CertRotator.TriggerCARotation != nil {
		readyCh := make(chan *mtls.TriggerResult)
		r.CertRotator.TriggerCARotation <- mtls.Trigger{Namespace: "", Ready: readyCh}
		<-readyCh
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *TerraformReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, maxConcurrentReconciles int, httpRetry int) error {
	// Index the Terraforms by the GitRepository references they (may) point at.
	if err := mgr.GetCache().IndexField(ctx, &infrav1.Terraform{}, infrav1.GitRepositoryIndexKey,
		r.IndexBy(sourcev1.GitRepositoryKind)); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	// Index the Terraforms by the Bucket references they (may) point at.
	if err := mgr.GetCache().IndexField(ctx, &infrav1.Terraform{}, infrav1.BucketIndexKey,
		r.IndexBy(sourcev1.BucketKind)); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	// Index the Terraforms by the OCIRepository references they (may) point at.
	if err := mgr.GetCache().IndexField(ctx, &infrav1.Terraform{}, infrav1.OCIRepositoryIndexKey,
		r.IndexBy(sourcev1.OCIRepositoryKind)); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	if r.EnableExternalArtifact {
		// Index the Terraforms by the ExternalArtifact references they (may) point at.
		if err := mgr.GetCache().IndexField(ctx, &infrav1.Terraform{}, infrav1.ExternalArtifactIndexKey,
			r.IndexBy(sourcev1.ExternalArtifactKind)); err != nil {
			return fmt.Errorf("failed setting index fields: %w", err)
		}
	}

	// Configure the retryable http client used for fetching artifacts.
	// By default, it retries 10 times within a 3.5 minutes window.
	httpClient := retryablehttp.NewClient()
	httpClient.RetryWaitMin = 5 * time.Second
	httpClient.RetryWaitMax = 30 * time.Second
	httpClient.RetryMax = httpRetry
	httpClient.Logger = nil
	r.httpClient = httpClient
	r.patchOptions = []patch.Option{patch.WithOwnedConditions{Conditions: infrav1.OwnedConditions}, patch.WithFieldOwner(r.FieldManager)}
	r.requeueDependency = 30 * time.Second
	recoverPanic := true

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.Terraform{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, predicates.ReconcileRequestedPredicate{}),
		)).
		Watches(
			&sourcev1.GitRepository{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForRevisionChangeOf(infrav1.GitRepositoryIndexKey)),
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		).
		Watches(
			&sourcev1.Bucket{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForRevisionChangeOf(infrav1.BucketIndexKey)),
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		).
		Watches(
			&sourcev1.OCIRepository{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForRevisionChangeOf(infrav1.OCIRepositoryIndexKey)),
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &infrav1.Terraform{}, handler.OnlyControllerOwner()),
			builder.WithPredicates(SecretDeletePredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: maxConcurrentReconciles,
			RecoverPanic:            &recoverPanic,
		})

	if r.EnableExternalArtifact {
		controllerBuilder = controllerBuilder.Watches(
			&sourcev1.ExternalArtifact{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForRevisionChangeOf(infrav1.ExternalArtifactIndexKey)),
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		)
	}

	return controllerBuilder.Complete(r)
}

func (r *TerraformReconciler) requestsForRevisionChangeOf(indexKey string) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)
		repo, ok := obj.(interface {
			GetArtifact() *meta.Artifact
		})
		if !ok {
			log.Error(fmt.Errorf("expected an object conformed with GetArtifact() method, but got a %T", obj), "failed to get reconcile requests for revision change")
			return nil
		}
		// If we do not have an artifact, we have no requests to make
		if repo.GetArtifact() == nil {
			return nil
		}

		var list infrav1.TerraformList
		if err := r.List(ctx, &list, client.MatchingFields{
			indexKey: client.ObjectKeyFromObject(obj).String(),
		}); err != nil {
			log.Error(err, "failed to list objects for revision change")
			return nil
		}
		var dd []dependency.Dependent
		for _, d := range list.Items {
			// If the revision of the artifact equals to the last attempted revision,
			// we should not make a request for this Terraform
			if repo.GetArtifact().Revision == d.Status.LastAttemptedRevision {
				continue
			}
			dd = append(dd, d.DeepCopy())
		}
		sorted, err := dependency.Sort(dd)
		if err != nil {
			log.Error(err, "failed to sort dependencies for revision change")
			return nil
		}
		reqs := make([]reconcile.Request, len(sorted))
		for i, t := range sorted {
			reqs[i].Name = t.Name
			reqs[i].Namespace = t.Namespace
		}
		return reqs
	}

}

func (r *TerraformReconciler) IndexBy(kind string) func(o client.Object) []string {
	return func(o client.Object) []string {
		terraform, ok := o.(*infrav1.Terraform)
		if !ok {
			panic(fmt.Sprintf("Expected a Terraform, got %T", o))
		}

		if terraform.Spec.SourceRef.Kind == kind {
			namespace := terraform.GetNamespace()
			if terraform.Spec.SourceRef.Namespace != "" {
				namespace = terraform.Spec.SourceRef.Namespace
			}
			return []string{fmt.Sprintf("%s/%s", namespace, terraform.Spec.SourceRef.Name)}
		}

		return nil
	}
}
