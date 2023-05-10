package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/fluxcd/pkg/runtime/logger"
	"github.com/fluxcd/pkg/runtime/metrics"
	"github.com/fluxcd/pkg/runtime/predicates"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	tfv1alpha2 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/mtls"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kuberecorder "k8s.io/client-go/tools/record"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// TerraformBranchBasesPlannerReconciler reconciles a Terraform object
type TerraformBranchBasesPlannerReconciler struct {
	client.Client
	httpClient        *retryablehttp.Client
	statusManager     string
	requeueDependency time.Duration

	EventRecorder   kuberecorder.EventRecorder
	MetricsRecorder *metrics.Recorder
	StatusPoller    *polling.StatusPoller
	Scheme          *runtime.Scheme
	CertRotator     *mtls.CertRotator
}

func (r *TerraformBranchBasesPlannerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reconcileStart := time.Now()
	reconciliationLoopID := uuid.New().String()
	log := ctrl.LoggerFrom(ctx, "reconciliation-loop-id", reconciliationLoopID, "start-time", reconcileStart)
	ctx = ctrl.LoggerInto(ctx, log)
	traceLog := log.V(logger.TraceLevel).WithValues("function", "TerraformBranchBasesPlannerReconciler.Reconcile")
	traceLog.Info("Reconcile Start")

	var terraform tfv1alpha2.Terraform
	if err := r.Get(ctx, req.NamespacedName, &terraform); err != nil {
		traceLog.Error(err, "Hit an error", "namespacedName", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.Info(fmt.Sprintf(">> Started Generation: %d", terraform.GetGeneration()))

	if !terraform.Spec.PlanOnly {
		// All branch-based terrafrom resources are PlanOnly. If it's not a PlanOnly
		// resource, skip it.
		return ctrl.Result{}, nil
	}

	// Do some magic.

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TerraformBranchBasesPlannerReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconciles int, httpRetry int) error {
	// Index the Terraforms by the GitRepository references they (may) point at.
	if err := mgr.GetCache().IndexField(context.TODO(), &tfv1alpha2.Terraform{}, tfv1alpha2.GitRepositoryIndexKey,
		IndexBy(sourcev1.GitRepositoryKind)); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	// Index the Terraforms by the Bucket references they (may) point at.
	if err := mgr.GetCache().IndexField(context.TODO(), &tfv1alpha2.Terraform{}, tfv1alpha2.BucketIndexKey,
		IndexBy(sourcev1b2.BucketKind)); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	// Index the Terraforms by the OCIRepository references they (may) point at.
	if err := mgr.GetCache().IndexField(context.TODO(), &tfv1alpha2.Terraform{}, tfv1alpha2.OCIRepositoryIndexKey,
		IndexBy(sourcev1b2.OCIRepositoryKind)); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	// Configure the retryable http client used for fetching artifacts.
	// By default, it retries 10 times within a 3.5 minutes window.
	httpClient := retryablehttp.NewClient()
	httpClient.RetryWaitMin = 5 * time.Second
	httpClient.RetryWaitMax = 30 * time.Second
	httpClient.RetryMax = httpRetry
	httpClient.Logger = nil
	r.httpClient = httpClient
	r.statusManager = "tf-bbp-controller"
	r.requeueDependency = 30 * time.Second
	recoverPanic := true

	return ctrl.NewControllerManagedBy(mgr).
		For(&tfv1alpha2.Terraform{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, predicates.ReconcileRequestedPredicate{}),
		)).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			&handler.EnqueueRequestForOwner{
				OwnerType:    &tfv1alpha2.Terraform{},
				IsController: true,
			},
			builder.WithPredicates(SecretDeletePredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: maxConcurrentReconciles,
			RecoverPanic:            &recoverPanic,
		}).
		Complete(r)
}
