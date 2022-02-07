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
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/events"
	"github.com/fluxcd/pkg/runtime/metrics"
	"github.com/fluxcd/pkg/runtime/predicates"
	"github.com/fluxcd/pkg/untar"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-exec/tfexec"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/mtls"
	"github.com/weaveworks/tf-controller/runner"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	kuberecorder "k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// TerraformReconciler reconciles a Terraform object
type TerraformReconciler struct {
	client.Client
	httpClient            *retryablehttp.Client
	EventRecorder         kuberecorder.EventRecorder
	ExternalEventRecorder *events.Recorder
	MetricsRecorder       *metrics.Recorder
	StatusPoller          *polling.StatusPoller
	Scheme                *runtime.Scheme
	CertRotator           *mtls.CertRotator
}

//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms/finalizers,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=buckets;gitrepositories,verbs=get;list;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=buckets/status;gitrepositories/status,verbs=get
// +kubebuilder:rbac:groups="",resources=configmaps;secrets;serviceaccounts,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Terraform object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *TerraformReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	reconcileStart := time.Now()

	var terraform infrav1.Terraform
	if err := r.Get(ctx, req.NamespacedName, &terraform); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Record suspended status metric
	defer r.recordSuspensionMetric(ctx, terraform)

	// Add our finalizer if it does not exist
	if !controllerutil.ContainsFinalizer(&terraform, infrav1.TerraformFinalizer) {
		controllerutil.AddFinalizer(&terraform, infrav1.TerraformFinalizer)
		if err := r.Update(ctx, &terraform); err != nil {
			log.Error(err, "unable to register finalizer")
			return ctrl.Result{}, err
		}
	}

	// TODO create Runner Pod
	// TODO wait for the Runner Pod to start
	runnerClient, err := r.LookupOrCreateRunner(ctx, terraform)
	if err != nil {
		panic(err.Error())
	}

	// Examine if the object is under deletion
	if !terraform.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, terraform, runnerClient)
	}

	// Return early if the Terraform is suspended.
	if terraform.Spec.Suspend {
		log.Info("Reconciliation is suspended for this object")
		return ctrl.Result{}, nil
	}

	// resolve source reference
	sourceObj, err := r.getSource(ctx, terraform)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("Source '%s' not found", terraform.Spec.SourceRef.String())
			terraform = infrav1.TerraformNotReady(terraform, "", infrav1.ArtifactFailedReason, msg)
			if err := r.patchStatus(ctx, req.NamespacedName, terraform.Status); err != nil {
				log.Error(err, "unable to update status for source not found")
				return ctrl.Result{Requeue: true}, err
			}
			r.recordReadinessMetric(ctx, terraform)
			log.Info(msg)
			// do not requeue immediately, when the source is created the watcher should trigger a reconciliation
			return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
		} else {
			// retry on transient errors
			return ctrl.Result{Requeue: true}, err
		}
	}

	if sourceObj.GetArtifact() == nil {
		msg := "Source is not ready, artifact not found"
		terraform = infrav1.TerraformNotReady(terraform, "", infrav1.ArtifactFailedReason, msg)
		if err := r.patchStatus(ctx, req.NamespacedName, terraform.Status); err != nil {
			log.Error(err, "unable to update status for artifact not found")
			return ctrl.Result{Requeue: true}, err
		}
		r.recordReadinessMetric(ctx, terraform)
		log.Info(msg)
		// do not requeue immediately, when the artifact is created the watcher should trigger a reconciliation
		return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
	}

	// If revision is changed, and there's no intend to apply,
	// we should clear the Pending Plan to trigger re-plan
	if sourceObj.GetArtifact().Revision != terraform.Status.LastAttemptedRevision && !r.shouldApply(terraform) {
		terraform.Status.Plan.Pending = ""
		if err := r.Status().Update(ctx, &terraform); err != nil {
			log.Error(err, "unable to update status to clear pending plan (revision != last attempted)")
			return ctrl.Result{Requeue: true}, err
		}
	}

	// Return early if it's manually mode and pending
	if terraform.Status.Plan.Pending != "" && !r.forceOrAutoApply(terraform) && !r.shouldApply(terraform) {
		log.Info("reconciliation is stopped to wait for a manual approve")
		return ctrl.Result{}, nil
	}

	// reconcile Terraform by applying the latest revision
	reconciledTerraform, reconcileErr := r.reconcile(ctx, runnerClient, *terraform.DeepCopy(), sourceObj)
	if err := r.patchStatus(ctx, req.NamespacedName, reconciledTerraform.Status); err != nil {
		log.Error(err, "unable to update status after reconciliation")
		return ctrl.Result{Requeue: true}, err
	}
	r.recordReadinessMetric(ctx, reconciledTerraform)

	if reconcileErr != nil && reconcileErr.Error() == infrav1.DriftDetectedReason {
		log.Error(reconcileErr, fmt.Sprintf("Drift detected after %s, next try in %s",
			time.Since(reconcileStart).String(),
			terraform.GetRetryInterval().String()),
			"revision",
			sourceObj.GetArtifact().Revision)
		r.fireEvent(ctx, reconciledTerraform, sourceObj.GetArtifact().Revision, events.EventSeverityError, reconcileErr.Error(), nil)
		return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
	} else if reconcileErr != nil {
		// broadcast the reconciliation failure and requeue at the specified retry interval
		log.Error(reconcileErr, fmt.Sprintf("Reconciliation failed after %s, next try in %s",
			time.Since(reconcileStart).String(),
			terraform.GetRetryInterval().String()),
			"revision",
			sourceObj.GetArtifact().Revision)
		r.fireEvent(ctx, reconciledTerraform, sourceObj.GetArtifact().Revision, events.EventSeverityError, reconcileErr.Error(), nil)
		return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
	}

	if reconciledTerraform.Status.Plan.Pending != "" && !r.forceOrAutoApply(reconciledTerraform) {
		log.Info("Reconciliation is stopped to wait for a manual approve")
		return ctrl.Result{}, nil
	}

	// next reconcile is .Spec.Interval in the future
	return ctrl.Result{RequeueAfter: terraform.Spec.Interval.Duration}, nil
}

func (r *TerraformReconciler) shouldDetectDrift(terraform infrav1.Terraform, revision string) bool {
	// return false when drift detection is disabled
	if terraform.Spec.DisableDriftDetection == true {
		return false
	}

	// not support when Destroy == true
	if terraform.Spec.Destroy == true {
		return false
	}

	// new object
	if terraform.Status.LastAppliedRevision == "" &&
		terraform.Status.LastPlannedRevision == "" &&
		terraform.Status.LastAttemptedRevision == "" {
		return false
	}

	// thing worked normally, no change pending
	// then, we do drift detection
	if terraform.Status.LastAttemptedRevision == terraform.Status.LastAppliedRevision &&
		terraform.Status.LastAttemptedRevision == terraform.Status.LastPlannedRevision &&
		terraform.Status.LastAttemptedRevision == revision &&
		terraform.Status.Plan.Pending == "" {
		return true
	}

	// last time source changed with non-TF file, so we planned but no changes
	// this time, it needs drift detection
	if terraform.Status.LastAttemptedRevision == terraform.Status.LastPlannedRevision &&
		terraform.Status.LastAttemptedRevision == revision &&
		terraform.Status.Plan.Pending == "" {
		return true
	}

	return false
}

func (r *TerraformReconciler) forceOrAutoApply(terraform infrav1.Terraform) bool {
	return terraform.Spec.Force || terraform.Spec.ApprovePlan == infrav1.ApprovePlanAutoValue
}

func (r *TerraformReconciler) shouldPlan(terraform infrav1.Terraform) bool {
	// Do not optimize this. We'll add other criteria later to infer plan actions
	if terraform.Spec.Force {
		return true
	}

	if terraform.Status.Plan.Pending == "" {
		return true
	} else if terraform.Status.Plan.Pending != "" {
		return false
	}
	return false
}

func (r *TerraformReconciler) shouldApply(terraform infrav1.Terraform) bool {
	// Do no optimize this logic, as we'd like to understand the explanation of the behaviour.
	if terraform.Spec.Force {
		return true
	}

	if terraform.Spec.ApprovePlan == "" {
		return false
	} else if terraform.Spec.ApprovePlan == infrav1.ApprovePlanAutoValue && terraform.Status.Plan.Pending != "" {
		return true
	} else if terraform.Spec.ApprovePlan == terraform.Status.Plan.Pending {
		return true
	} else if strings.HasPrefix(terraform.Status.Plan.Pending, terraform.Spec.ApprovePlan) {
		return true
	}
	return false
}

func (r *TerraformReconciler) shouldWriteOutputs(terraform infrav1.Terraform, outputs map[string]tfexec.OutputMeta) bool {
	if terraform.Spec.WriteOutputsToSecret != nil && len(outputs) > 0 {
		return true
	}

	return false
}

func (r *TerraformReconciler) shouldDoHealthChecks(terraform infrav1.Terraform) bool {
	if terraform.Spec.HealthChecks == nil || len(terraform.Spec.HealthChecks) < 1 {
		return false
	}

	var applyCondition metav1.Condition
	var hcCondition metav1.Condition
	for _, c := range terraform.Status.Conditions {
		if c.Type == "Apply" {
			applyCondition = c
		} else if c.Type == "HealthCheck" {
			hcCondition = c
		}
	}

	// health checks were previously performed but failed
	// do health check again
	if hcCondition.Reason == infrav1.HealthChecksFailedReason {
		return true
	}

	// terraform was applied and no health check performed yet
	// do health check
	if applyCondition.Reason == infrav1.TFExecApplySucceedReason &&
		hcCondition.Reason == "" {
		return true
	}

	return false
}

type LocalPrintfer struct {
	logger logr.Logger
}

func (l LocalPrintfer) Printf(format string, v ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, v...))
}

func (r *TerraformReconciler) reconcile(ctx context.Context, runnerClient runner.RunnerClient, terraform infrav1.Terraform, sourceObj sourcev1.Source) (infrav1.Terraform, error) {

	log := ctrl.LoggerFrom(ctx)
	revision := sourceObj.GetArtifact().Revision
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	terraform = infrav1.TerraformProgressing(terraform, "Initializing")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "unable to update status before Terraform initialization")
		return terraform, err
	}

	// download artifact and extract files
	buf, err := r.downloadAsBytes(sourceObj.GetArtifact())
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.ArtifactFailedReason,
			err.Error(),
		), err
	}

	uploadAndExtractReply, err := runnerClient.UploadAndExtract(ctx, &runner.UploadAndExtractRequest{
		Namespace: terraform.Namespace,
		Name:      terraform.Name,
		TarGz:     buf.Bytes(),
		Path:      terraform.Spec.Path,
	})
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.ArtifactFailedReason,
			err.Error(),
		), err
	}
	workingDir := uploadAndExtractReply.WorkingDir
	tmpDir := uploadAndExtractReply.TmpDir

	defer func() {
		cleanupDirReply, err := runnerClient.CleanupDir(ctx, &runner.CleanupDirRequest{TmpDir: tmpDir})
		if err != nil {
			log.Error(err, "clean up error")
		}
		if cleanupDirReply != nil {
			log.Info(fmt.Sprintf("clean up dir: %s", cleanupDirReply.Message))
		}
	}()

	var backendConfig string
	DisableTFK8SBackend := os.Getenv("DISABLE_TF_K8S_BACKEND") == "1"

	if terraform.Spec.BackendConfig != nil {
		backendConfig = fmt.Sprintf(`
terraform {
  backend "kubernetes" {
    secret_suffix     = "%s"
    in_cluster_config = %v
    config_path       = "%s"
    namespace         = "%s"
  }
}
`,
			terraform.Spec.BackendConfig.SecretSuffix,
			terraform.Spec.BackendConfig.InClusterConfig,
			terraform.Spec.BackendConfig.ConfigPath,
			terraform.Namespace)
	} else if DisableTFK8SBackend && terraform.Spec.BackendConfig == nil {
		backendConfig = `
terraform {
	backend "local" { }
}`
	} else if terraform.Spec.BackendConfig == nil {
		// TODO must be tested in cluster only
		backendConfig = fmt.Sprintf(`
terraform {
  backend "kubernetes" {
    secret_suffix     = "%s"
    in_cluster_config = true
    namespace         = "%s"
  }
}
`, terraform.Name, terraform.Namespace)
	}

	if r.backendCompletelyDisable(terraform) {
		log.Info("BackendConfig is completely disabled")
	} else {
		writeBackendConfigReply, err := runnerClient.WriteBackendConfig(ctx,
			&runner.WriteBackendConfigRequest{
				DirPath:       workingDir,
				BackendConfig: []byte(backendConfig),
			})

		log.Info(fmt.Sprintf("write backend config: %s", writeBackendConfigReply.Message))
		if err != nil {
			return terraform, err
		}
	}

	var tfrcFilepath string
	if terraform.Spec.CliConfigSecretRef != nil {
		cliConfigSecretRef := *(terraform.Spec.CliConfigSecretRef.DeepCopy())
		if cliConfigSecretRef.Namespace == "" {
			cliConfigSecretRef.Namespace = terraform.Namespace
		}

		processCliConfigReply, err := runnerClient.ProcessCliConfig(ctx, &runner.ProcessCliConfigRequest{
			DirPath:   workingDir,
			Namespace: cliConfigSecretRef.Namespace,
			Name:      cliConfigSecretRef.Name,
		})
		if err != nil {
			err = fmt.Errorf("cannot process cli config: %s", err.Error())
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.TFExecNewFailedReason,
				err.Error(),
			), err
		}
		tfrcFilepath = processCliConfigReply.FilePath
	}

	lookPathReply, err := runnerClient.LookPath(ctx,
		&runner.LookPathRequest{
			File: "terraform",
		})
	execPath := lookPathReply.ExecPath
	if err != nil {
		err = fmt.Errorf("cannot find Terraform binary: %s in %s", err, os.Getenv("PATH"))
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecNewFailedReason,
			err.Error(),
		), err
	}

	newTerraformReply, err := runnerClient.NewTerraform(ctx,
		&runner.NewTerraformRequest{
			// TarGz:      tarGzBytes,
			WorkingDir: workingDir,
			ExecPath:   execPath,
		})
	if err != nil {
		err = fmt.Errorf("error running NewTerraform: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecNewFailedReason,
			err.Error(),
		), err
	}

	tfInstance := newTerraformReply.Id
	envs := map[string]string{}

	disableTestLogging := os.Getenv("DISABLE_TF_LOGS") == "1"
	if !disableTestLogging {
		envs["DISABLE_TF_LOGS"] = "1"
	}

	if tfrcFilepath != "" {
		envs["TF_CLI_CONFIG_FILE"] = tfrcFilepath
		if setEnvReply, err := runnerClient.SetEnv(ctx,
			&runner.SetEnvRequest{
				TfInstance: tfInstance,
				Envs:       envs,
			}); err != nil {
			err = fmt.Errorf("error setting env for Terraform: %s %s", err, setEnvReply.Message)
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.TFExecInitFailedReason,
				err.Error(),
			), err
		}
	}

	log.Info("new terraform", "workingDir", workingDir)

	// TODO we currently use a fork version of TFExec to workaround the forceCopy bug
	// https://github.com/hashicorp/terraform-exec/issues/262
	/*
		initOpts := []tfexec.InitOption{tfexec.Upgrade(true), tfexec.ForceCopy(true)}
		if r.backendCompletelyDisable(terraform) {
			initOpts = append(initOpts, tfexec.ForceCopy(false))
		}
	*/

	initRequest := &runner.InitRequest{
		TfInstance: tfInstance,
		Upgrade:    true,
		ForceCopy:  true,
	}
	if r.backendCompletelyDisable(terraform) {
		initRequest.ForceCopy = false
	}

	initReply, err := runnerClient.Init(ctx, initRequest)
	if err != nil {
		err = fmt.Errorf("error running Init: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecInitFailedReason,
			err.Error(),
		), err
	}
	log.Info(fmt.Sprintf("init reply: %s", initReply.Message))

	log.Info("tfexec initialized terraform")

	terraformBytes, err := terraform.ToBytes(r.Scheme)
	if err != nil {
		// transient error?
		return terraform, err
	}
	generateVarsForTFReply, err := runnerClient.GenerateVarsForTF(ctx, &runner.GenerateVarsForTFRequest{
		Terraform:  terraformBytes,
		WorkingDir: workingDir,
	})
	if err != nil {
		// transient error?
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.VarsGenerationFailedReason,
			err.Error(),
		), err
	}
	log.Info(fmt.Sprintf("generate vars from tf: %s", generateVarsForTFReply.Message))

	log.Info("generated var files from spec")

	if r.shouldDetectDrift(terraform, revision) {
		var driftDetectionErr error // declared here to avoid shadowing on terraform variable
		terraform, driftDetectionErr = r.detectDrift(ctx, terraform, tfInstance, runnerClient, revision)

		// immediately return if no drift - reconciliation will retry normally
		if driftDetectionErr == nil {
			return terraform, nil
		}

		// immediately return if err is not about drift
		if driftDetectionErr.Error() != infrav1.DriftDetectedReason {
			return terraform, driftDetectionErr
		}

		// immediately return if drift is detected but it's not "force" or "auto"
		if driftDetectionErr.Error() == infrav1.DriftDetectedReason && !r.forceOrAutoApply(terraform) {
			return terraform, driftDetectionErr
		}

		// ok, patch and continue
		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after drift detection")
			return terraform, err
		}
	}

	// return early if we're in drift-detection-only mode
	if terraform.Spec.ApprovePlan == infrav1.ApprovePlanDisableValue {
		log.Info("approve plan disabled")
		return terraform, nil
	}

	if r.shouldPlan(terraform) {
		terraform, err = r.plan(ctx, terraform, tfInstance, runnerClient, revision)
		if err != nil {
			return terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after planing")
			return terraform, err
		}
	}

	outputs := map[string]tfexec.OutputMeta{}
	if r.shouldApply(terraform) {
		terraform, err = r.apply(ctx, terraform, tfInstance, runnerClient, revision, &outputs)
		if err != nil {
			return terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after applying")
			return terraform, err
		}
	} else {
		log.Info("should apply == false")
	}

	if r.shouldWriteOutputs(terraform, outputs) {
		terraform, err = r.writeOutput(ctx, terraform, runnerClient, outputs, revision)
		if err != nil {
			return terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after writing outputs")
			return terraform, err
		}
	}

	if r.shouldDoHealthChecks(terraform) {
		terraform, err = r.doHealthChecks(ctx, terraform, runnerClient)
		if err != nil {
			return terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after doing health checks")
			return terraform, err
		}
	}

	return terraform, nil
}

func (r *TerraformReconciler) detectDrift(ctx context.Context, terraform infrav1.Terraform, tfInstance string, runnerClient runner.RunnerClient, revision string) (infrav1.Terraform, error) {

	log := ctrl.LoggerFrom(ctx)

	log.Info("calling detectDrift ...")

	const (
		driftFilename = "tfdrift"
	)

	planRequest := &runner.PlanRequest{
		TfInstance: tfInstance,
		Out:        driftFilename,
		Refresh:    true,
	}
	if r.backendCompletelyDisable(terraform) {
		planRequest.Out = ""
		planRequest.Refresh = true
	}

	planReply, err := runnerClient.Plan(ctx, planRequest)
	if err != nil {
		err = fmt.Errorf("error running Plan: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.DriftDetectionFailedReason,
			err.Error(),
		), err
	}
	drifted := planReply.Drifted
	log.Info(fmt.Sprintf("plan for drift: %s found drift: %v", planReply.Message, planReply.Drifted))

	if drifted {
		var rawOutput string
		if r.backendCompletelyDisable(terraform) {
			rawOutput = "not available"
		} else {
			showPlanFileRawReply, err := runnerClient.ShowPlanFileRaw(ctx, &runner.ShowPlanFileRawRequest{
				TfInstance: tfInstance,
				Filename:   driftFilename,
			})
			if err != nil {
				return infrav1.TerraformNotReady(
					terraform,
					revision,
					infrav1.DriftDetectionFailedReason,
					err.Error(),
				), err
			}
			rawOutput = showPlanFileRawReply.RawOutput
			log.Info(fmt.Sprintf("show plan: %s", showPlanFileRawReply.RawOutput))
		}

		// If drift detected & we use the auto mode, then we continue
		terraform = infrav1.TerraformDriftDetected(terraform, revision, infrav1.DriftDetectedReason, rawOutput)
		return terraform, fmt.Errorf(infrav1.DriftDetectedReason)
	}

	terraform = infrav1.TerraformNoDrift(terraform, revision, infrav1.NoDriftReason, "No drift")
	return terraform, nil
}

func (r *TerraformReconciler) plan(ctx context.Context, terraform infrav1.Terraform, tfInstance string, runnerClient runner.RunnerClient, revision string) (infrav1.Terraform, error) {

	log := ctrl.LoggerFrom(ctx)

	log.Info("calling plan ...")

	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}
	terraform = infrav1.TerraformProgressing(terraform, "Terraform Planning")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "unable to update status before Terraform planning")
		return terraform, err
	}

	const tfplanFilename = "tfplan"

	planRequest := &runner.PlanRequest{
		TfInstance: tfInstance,
		Out:        tfplanFilename,
		Refresh:    true, // be careful, refresh requires to be true by default
	}

	if r.backendCompletelyDisable(terraform) {
		planRequest.Out = ""
	}

	if terraform.Spec.Destroy {
		log.Info("spec.destroy is set")
		planRequest.Destroy = true
	}

	planReply, err := runnerClient.Plan(ctx, planRequest)
	if err != nil {
		err = fmt.Errorf("error running Plan: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecPlanFailedReason,
			err.Error(),
		), err
	}
	drifted := planReply.Drifted
	log.Info(fmt.Sprintf("plan: %s, found drift: %v", planReply.Message, drifted))

	saveTFPlanReply, err := runnerClient.SaveTFPlan(ctx, &runner.SaveTFPlanRequest{
		TfInstance:               tfInstance,
		BackendCompletelyDisable: r.backendCompletelyDisable(terraform),
		Name:                     terraform.Name,
		Namespace:                terraform.Namespace,
		Uuid:                     string(terraform.GetUID()),
		Revision:                 revision,
	})
	if err != nil {
		err = fmt.Errorf("error saving plan secret: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecPlanFailedReason,
			err.Error(),
		), err
	}
	log.Info(fmt.Sprintf("save tfplan: %s", saveTFPlanReply.Message))

	if drifted {
		terraform = infrav1.TerraformPlannedWithChanges(terraform, revision, "Plan generated")
	} else {
		terraform = infrav1.TerraformPlannedNoChanges(terraform, revision, "Plan no changes")
	}

	return terraform, nil
}

func (r *TerraformReconciler) backendCompletelyDisable(terraform infrav1.Terraform) bool {
	return terraform.Spec.BackendConfig != nil && terraform.Spec.BackendConfig.Disable == true
}

func (r *TerraformReconciler) apply(ctx context.Context, terraform infrav1.Terraform, tfInstance string, runnerClient runner.RunnerClient, revision string, outputs *map[string]tfexec.OutputMeta) (infrav1.Terraform, error) {

	const (
		TFPlanName = "tfplan"
	)

	log := ctrl.LoggerFrom(ctx)
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	terraform = infrav1.TerraformProgressing(terraform, "Applying")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "unable to update status before Terraform applying")
		return terraform, err
	}

	loadTFPlanReply, err := runnerClient.LoadTFPlan(ctx, &runner.LoadTFPlanRequest{
		TfInstance:               tfInstance,
		Name:                     terraform.Name,
		Namespace:                terraform.Namespace,
		BackendCompletelyDisable: r.backendCompletelyDisable(terraform),
		PendingPlan:              terraform.Status.Plan.Pending,
	})
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecApplyFailedReason,
			err.Error(),
		), err
	}

	log.Info(fmt.Sprintf("load tf plan: %s", loadTFPlanReply.Message))

	terraform = infrav1.TerraformApplying(terraform, revision, "Apply started")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "error recording apply status: %s", err)
		return terraform, err
	}

	applyRequest := &runner.ApplyRequest{
		TfInstance: tfInstance,
	}
	if r.backendCompletelyDisable(terraform) {
		// do nothing
	} else {
		applyRequest.DirOrPlan = TFPlanName
	}

	var isDestroyApplied bool
	// this a special case, when backend is completely disabled.
	// we need to use "destroy" command instead of apply
	if r.backendCompletelyDisable(terraform) && terraform.Spec.Destroy == true {
		destroyReply, err := runnerClient.Destroy(ctx, &runner.DestroyRequest{
			TfInstance: tfInstance,
		})
		log.Info(fmt.Sprintf("destroy: %s", destroyReply.Message))
		if err != nil {
			err = fmt.Errorf("error running Destroy: %s", err)
			return infrav1.TerraformAppliedFailResetPlanAndNotReady(
				terraform,
				revision,
				infrav1.TFExecApplyFailedReason,
				err.Error(),
			), err
		}
		isDestroyApplied = true
	} else {
		applyReply, err := runnerClient.Apply(ctx, applyRequest)
		if err != nil {
			err = fmt.Errorf("error running Apply: %s", err)
			return infrav1.TerraformAppliedFailResetPlanAndNotReady(
				terraform,
				revision,
				infrav1.TFExecApplyFailedReason,
				err.Error(),
			), err
		}
		log.Info(fmt.Sprintf("apply: %s", applyReply.Message))

		isDestroyApplied = terraform.Status.Plan.IsDestroyPlan
	}

	terraform = infrav1.TerraformApplied(terraform, revision, "Applied successfully", isDestroyApplied)

	outputReply, err := runnerClient.Output(ctx, &runner.OutputRequest{
		TfInstance: tfInstance,
	})
	if err != nil {
		// TODO should not be this Error
		// warning-like status is enough
		err = fmt.Errorf("error running Output: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecOutputFailedReason,
			err.Error(),
		), err
	}
	*outputs = convertOutputs(outputReply.Outputs)

	var availableOutputs []string
	for k := range *outputs {
		availableOutputs = append(availableOutputs, k)
	}
	if len(availableOutputs) > 0 {
		sort.Strings(availableOutputs)
		terraform = infrav1.TerraformOutputsAvailable(terraform, availableOutputs, "Outputs available")
	}

	return terraform, nil
}

func convertOutputs(outputs map[string]*runner.OutputMeta) map[string]tfexec.OutputMeta {
	result := map[string]tfexec.OutputMeta{}
	for k, v := range outputs {
		result[k] = tfexec.OutputMeta{
			Sensitive: v.Sensitive,
			Type:      v.Type,
			Value:     v.Value,
		}
	}
	return result
}

func (r *TerraformReconciler) writeOutput(ctx context.Context, terraform infrav1.Terraform, runnerClient runner.RunnerClient, outputs map[string]tfexec.OutputMeta, revision string) (infrav1.Terraform, error) {
	type hcl struct {
		Name  string    `cty:"name"`
		Value cty.Value `cty:"value"`
	}

	log := ctrl.LoggerFrom(ctx)

	wots := terraform.Spec.WriteOutputsToSecret
	data := map[string][]byte{}

	// if not specified .spec.writeOutputsToSecret.outputs,
	// then it means export all outputs
	if len(wots.Outputs) == 0 {
		for output, v := range outputs {
			ct, err := ctyjson.UnmarshalType(v.Type)
			if err != nil {
				return terraform, err
			}
			// if it's a string, we can embed it directly into Secret's data
			switch ct {
			case cty.String:
				cv, err := ctyjson.Unmarshal(v.Value, ct)
				if err != nil {
					return terraform, err
				}
				data[output] = []byte(cv.AsString())
			// there's no need to unmarshal and convert to []byte
			// we'll just pass the []byte directly from OutputMeta Value
			case cty.Number, cty.Bool:
				data[output] = v.Value
			default:
				outputBytes, err := json.Marshal(v.Value)
				if err != nil {
					return terraform, err
				}
				data[output] = outputBytes
			}
		}
	} else {
		// filter only defined output
		for _, output := range wots.Outputs {
			v := outputs[output]
			ct, err := ctyjson.UnmarshalType(v.Type)
			if err != nil {
				return terraform, err
			}
			switch ct {
			case cty.String:
				cv, err := ctyjson.Unmarshal(v.Value, ct)
				if err != nil {
					return terraform, err
				}
				data[output] = []byte(cv.AsString())
			// there's no need to unmarshal and convert to []byte
			// we'll just pass the []byte directly from OutputMeta Value
			case cty.Number, cty.Bool:
				data[output] = v.Value
			default:
				outputBytes, err := json.Marshal(v.Value)
				if err != nil {
					return terraform, err
				}
				data[output] = outputBytes
			}
		}
	}

	if len(data) == 0 || terraform.Spec.Destroy == true {
		return infrav1.TerraformOutputsWritten(terraform, revision, "No Outputs written"), nil
	}

	writeOutputsReply, err := runnerClient.WriteOutputs(ctx, &runner.WriteOutputsRequest{
		Namespace:  terraform.Namespace,
		Name:       terraform.Name,
		SecretName: terraform.Spec.WriteOutputsToSecret.Name,
		Uuid:       string(terraform.UID),
		Data:       data,
	})
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.OutputsWritingFailedReason,
			err.Error(),
		), err
	}
	log.Info(fmt.Sprintf("write outputs: %s", writeOutputsReply.Message))

	return infrav1.TerraformOutputsWritten(terraform, revision, "Outputs written"), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TerraformReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconciles int, httpRetry int) error {
	const (
		gitRepositoryIndexKey string = ".metadata.gitRepository"
		bucketIndexKey        string = ".metadata.bucket"
	)

	// Index the Terraforms by the GitRepository references they (may) point at.
	if err := mgr.GetCache().IndexField(context.TODO(), &infrav1.Terraform{}, gitRepositoryIndexKey,
		r.indexBy(sourcev1.GitRepositoryKind)); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	// Index the Terraforms by the Bucket references they (may) point at.
	if err := mgr.GetCache().IndexField(context.TODO(), &infrav1.Terraform{}, bucketIndexKey,
		r.indexBy(sourcev1.BucketKind)); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	// Configure the retryable http client used for fetching artifacts.
	// By default it retries 10 times within a 3.5 minutes window.
	httpClient := retryablehttp.NewClient()
	httpClient.RetryWaitMin = 5 * time.Second
	httpClient.RetryWaitMax = 30 * time.Second
	httpClient.RetryMax = httpRetry
	httpClient.Logger = nil
	r.httpClient = httpClient

	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.Terraform{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, predicates.ReconcileRequestedPredicate{}),
		)).
		Watches(
			&source.Kind{Type: &sourcev1.GitRepository{}},
			handler.EnqueueRequestsFromMapFunc(r.requestsForRevisionChangeOf(gitRepositoryIndexKey)),
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		).
		Watches(
			&source.Kind{Type: &sourcev1.Bucket{}},
			handler.EnqueueRequestsFromMapFunc(r.requestsForRevisionChangeOf(bucketIndexKey)),
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrentReconciles}).
		Complete(r)
}

func (r *TerraformReconciler) requestsForRevisionChangeOf(indexKey string) func(obj client.Object) []reconcile.Request {
	return func(obj client.Object) []reconcile.Request {
		repo, ok := obj.(interface {
			GetArtifact() *sourcev1.Artifact
		})
		if !ok {
			panic(fmt.Sprintf("Expected an object conformed with GetArtifact() method, but got a %T", obj))
		}
		// If we do not have an artifact, we have no requests to make
		if repo.GetArtifact() == nil {
			return nil
		}

		ctx := context.Background()
		var list infrav1.TerraformList
		if err := r.List(ctx, &list, client.MatchingFields{
			indexKey: client.ObjectKeyFromObject(obj).String(),
		}); err != nil {
			return nil
		}
		reqs := make([]reconcile.Request, len(list.Items))
		for i, t := range list.Items {
			// If the revision of the artifact equals to the last attempted revision,
			// we should not make a request for this Terraform
			if repo.GetArtifact().Revision == t.Status.LastAttemptedRevision {
				continue
			}
			reqs[i].NamespacedName.Name = t.Name
			reqs[i].NamespacedName.Namespace = t.Namespace
		}
		return reqs
	}

}

func (r *TerraformReconciler) getSource(ctx context.Context, terraform infrav1.Terraform) (sourcev1.Source, error) {
	var sourceObj sourcev1.Source
	sourceNamespace := terraform.GetNamespace()
	if terraform.Spec.SourceRef.Namespace != "" {
		sourceNamespace = terraform.Spec.SourceRef.Namespace
	}
	namespacedName := types.NamespacedName{
		Namespace: sourceNamespace,
		Name:      terraform.Spec.SourceRef.Name,
	}
	switch terraform.Spec.SourceRef.Kind {
	case sourcev1.GitRepositoryKind:
		var repository sourcev1.GitRepository
		err := r.Client.Get(ctx, namespacedName, &repository)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return sourceObj, err
			}
			return sourceObj, fmt.Errorf("unable to get source '%s': %w", namespacedName, err)
		}
		sourceObj = &repository
	case sourcev1.BucketKind:
		var bucket sourcev1.Bucket
		err := r.Client.Get(ctx, namespacedName, &bucket)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return sourceObj, err
			}
			return sourceObj, fmt.Errorf("unable to get source '%s': %w", namespacedName, err)
		}
		sourceObj = &bucket
	default:
		return sourceObj, fmt.Errorf("source `%s` kind '%s' not supported",
			terraform.Spec.SourceRef.Name, terraform.Spec.SourceRef.Kind)
	}
	return sourceObj, nil
}

func (r *TerraformReconciler) downloadAsBytes(artifact *sourcev1.Artifact) (*bytes.Buffer, error) {
	artifactURL := artifact.URL
	if hostname := os.Getenv("SOURCE_CONTROLLER_LOCALHOST"); hostname != "" {
		u, err := url.Parse(artifactURL)
		if err != nil {
			return nil, err
		}
		u.Host = hostname
		artifactURL = u.String()
	}

	req, err := retryablehttp.NewRequest(http.MethodGet, artifactURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download artifact, error: %w", err)
	}
	defer resp.Body.Close()

	// check response
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download artifact from %s, status: %s", artifactURL, resp.Status)
	}

	var buf bytes.Buffer

	// verify checksum matches origin
	if err := r.verifyArtifact(artifact, &buf, resp.Body); err != nil {
		return nil, err
	}

	return &buf, nil
}

func (r *TerraformReconciler) downloadAndExtract(artifact *sourcev1.Artifact, tmpDir string) error {
	artifactURL := artifact.URL
	if hostname := os.Getenv("SOURCE_CONTROLLER_LOCALHOST"); hostname != "" {
		u, err := url.Parse(artifactURL)
		if err != nil {
			return err
		}
		u.Host = hostname
		artifactURL = u.String()
	}

	req, err := retryablehttp.NewRequest(http.MethodGet, artifactURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create a new request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download artifact, error: %w", err)
	}
	defer resp.Body.Close()

	// check response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download artifact from %s, status: %s", artifactURL, resp.Status)
	}

	var buf bytes.Buffer

	// verify checksum matches origin
	if err := r.verifyArtifact(artifact, &buf, resp.Body); err != nil {
		return err
	}

	// extract
	if _, err = untar.Untar(&buf, tmpDir); err != nil {
		return fmt.Errorf("failed to untar artifact, error: %w", err)
	}

	return nil
}

func (r *TerraformReconciler) recordReadinessMetric(ctx context.Context, terraform infrav1.Terraform) {
	if r.MetricsRecorder == nil {
		return
	}
	log := ctrl.LoggerFrom(ctx)

	objRef, err := reference.GetReference(r.Scheme, &terraform)
	if err != nil {
		log.Error(err, "unable to record readiness metric")
		return
	}
	if rc := apimeta.FindStatusCondition(terraform.Status.Conditions, meta.ReadyCondition); rc != nil {
		r.MetricsRecorder.RecordCondition(*objRef, *rc,
			!terraform.DeletionTimestamp.IsZero())
	} else {
		r.MetricsRecorder.RecordCondition(*objRef, metav1.Condition{
			Type:   meta.ReadyCondition,
			Status: metav1.ConditionUnknown,
		}, !terraform.DeletionTimestamp.IsZero())
	}
}

func (r *TerraformReconciler) recordSuspensionMetric(ctx context.Context, terraform infrav1.Terraform) {
	if r.MetricsRecorder == nil {
		return
	}
	log := ctrl.LoggerFrom(ctx)

	objRef, err := reference.GetReference(r.Scheme, &terraform)
	if err != nil {
		log.Error(err, "unable to record suspended metric")
		return
	}

	if !terraform.DeletionTimestamp.IsZero() {
		r.MetricsRecorder.RecordSuspend(*objRef, false)
	} else {
		r.MetricsRecorder.RecordSuspend(*objRef, terraform.Spec.Suspend)
	}
}

func (r *TerraformReconciler) patchStatus(ctx context.Context, objectKey types.NamespacedName, newStatus infrav1.TerraformStatus) error {
	var terraform infrav1.Terraform
	if err := r.Get(ctx, objectKey, &terraform); err != nil {
		return err
	}

	patch := client.MergeFrom(terraform.DeepCopy())
	terraform.Status = newStatus

	return r.Status().Patch(ctx, &terraform, patch)
}

func (r *TerraformReconciler) verifyArtifact(artifact *sourcev1.Artifact, buf *bytes.Buffer, reader io.Reader) error {
	hasher := sha256.New()

	// for backwards compatibility with source-controller v0.17.2 and older
	if len(artifact.Checksum) == 40 {
		hasher = sha1.New()
	}

	// compute checksum
	mw := io.MultiWriter(hasher, buf)
	if _, err := io.Copy(mw, reader); err != nil {
		return err
	}

	if checksum := fmt.Sprintf("%x", hasher.Sum(nil)); checksum != artifact.Checksum {
		return fmt.Errorf("failed to verify artifact: computed checksum '%s' doesn't match advertised '%s'",
			checksum, artifact.Checksum)
	}

	return nil
}

func (r *TerraformReconciler) indexBy(kind string) func(o client.Object) []string {
	return func(o client.Object) []string {
		terraform, ok := o.(*infrav1.Terraform)
		if !ok {
			panic(fmt.Sprintf("Expected a Kustomization, got %T", o))
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

func (r *TerraformReconciler) fireEvent(ctx context.Context, terraform infrav1.Terraform, revision, severity, msg string, metadata map[string]string) {
	log := ctrl.LoggerFrom(ctx)

	annotations := map[string]string{
		infrav1.GroupVersion.Group + "/revision": revision,
	}

	eventType := "Normal"
	if severity == events.EventSeverityError {
		eventType = "Warning"
	}

	r.EventRecorder.AnnotatedEventf(&terraform, annotations, eventType, severity, msg)

	if r.ExternalEventRecorder != nil {
		objRef, err := reference.GetReference(r.Scheme, &terraform)
		if err != nil {
			log.Error(err, "unable to send event")
			return
		}
		if metadata == nil {
			metadata = map[string]string{}
		}
		if revision != "" {
			metadata["revision"] = revision
		}

		reason := severity
		if c := apimeta.FindStatusCondition(terraform.Status.Conditions, meta.ReadyCondition); c != nil {
			reason = c.Reason
		}

		if err := r.ExternalEventRecorder.Eventf(*objRef, metadata, severity, reason, msg); err != nil {
			log.Error(err, "unable to send event")
			return
		}
	}
}

func (r *TerraformReconciler) finalize(ctx context.Context, terraform infrav1.Terraform, runnerClient runner.RunnerClient) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	outputSecretName := ""
	hasSpecifiedOutputSecret := terraform.Spec.WriteOutputsToSecret != nil && terraform.Spec.WriteOutputsToSecret.Name != ""
	if hasSpecifiedOutputSecret {
		outputSecretName = terraform.Spec.WriteOutputsToSecret.Name
	}

	finalizeSecretsReply, err := runnerClient.FinalizeSecrets(ctx, &runner.FinalizeSecretsRequest{
		Namespace:                terraform.Namespace,
		Name:                     terraform.Name,
		HasSpecifiedOutputSecret: hasSpecifiedOutputSecret,
		OutputSecretName:         outputSecretName,
	})
	if err != nil {
		if e, ok := status.FromError(err); ok {
			switch e.Code() {
			case codes.Internal:
				// transient error
				return ctrl.Result{Requeue: true}, err
			case codes.NotFound:
				// do nothing, fall through
			}
		}
	}

	if err == nil {
		log.Info(fmt.Sprintf("finalizing secrets: %s", finalizeSecretsReply.Message))
	}

	// Record deleted status
	r.recordReadinessMetric(ctx, terraform)

	// Remove our finalizer from the list and update it
	controllerutil.RemoveFinalizer(&terraform, infrav1.TerraformFinalizer)
	if err := r.Update(ctx, &terraform); err != nil {
		return ctrl.Result{}, err
	}

	// Stop reconciliation as the object is being deleted
	return ctrl.Result{}, nil
}

func getPodFQN(pod corev1.Pod) (string, error) {
	podIP := pod.Status.PodIP
	if podIP == "" {
		return "", fmt.Errorf("pod ip not found")
	}
	prefix := strings.ReplaceAll(podIP, ".", "-")
	fqn := fmt.Sprintf("%s.%s.pod.%s", prefix, pod.Namespace, "cluster.local.")
	return fqn, nil
}

func (r *TerraformReconciler) LookupOrCreateRunner(ctx context.Context, terraform infrav1.Terraform) (runner.RunnerClient, error) {
	// Pod's IP pattern: http://10-244-0-7.default.pod.cluster.local
	if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
		conn, err := r.getRunnerConnection(ctx, &terraform, "localhost:30000")
		if err != nil {
			return nil, err
		}
		defer conn.Close()

		runnerClient := runner.NewRunnerClient(conn)
		return runnerClient, nil
	}

	tlsSecretName, err := r.reconcileRunnerSecret(ctx, &terraform)
	if err != nil {
		return nil, err
	}

	err = r.reconcileRunnerPod(ctx, &terraform, tlsSecretName)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (r *TerraformReconciler) doHealthChecks(ctx context.Context, terraform infrav1.Terraform, runnerClient runner.RunnerClient) (infrav1.Terraform, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("calling doHealthChecks ...")

	// get terraform output data for health check urls
	outputs := make(map[string]string)
	if terraform.Spec.WriteOutputsToSecret != nil && terraform.Spec.WriteOutputsToSecret.Name != "" {
		getOutputsReply, err := runnerClient.GetOutputs(ctx, &runner.GetOutputsRequest{
			Namespace:  terraform.Namespace,
			SecretName: terraform.Spec.WriteOutputsToSecret.Name,
		})
		if err != nil {
			err = fmt.Errorf("error getting terraform output for health checks: %s", err)
			return infrav1.TerraformHealthCheckFailed(
				terraform,
				err.Error(),
			), err
		}
		outputs = getOutputsReply.Outputs
	}

	for _, hc := range terraform.Spec.HealthChecks {
		// perform health check based on type
		switch hc.Type {
		case infrav1.HealthCheckTypeTCP:
			parsed, err := r.templateParse(outputs, hc.Address)
			if err != nil {
				err = fmt.Errorf("error getting terraform output for health checks: %s", err)
				return infrav1.TerraformHealthCheckFailed(
					terraform,
					err.Error(),
				), err
			}

			err = r.doTCPHealthCheck(ctx, hc.Name, parsed, hc.GetTimeout())
			if err != nil {
				return infrav1.TerraformHealthCheckFailed(
					terraform,
					err.Error(),
				), err
			}
		case infrav1.HealthCheckTypeHttpGet:
			parsed, err := r.templateParse(outputs, hc.URL)
			if err != nil {
				err = fmt.Errorf("error getting terraform output for health checks: %s", err)
				return infrav1.TerraformHealthCheckFailed(
					terraform,
					err.Error(),
				), err
			}

			err = r.doHTTPHealthCheck(ctx, hc.Name, parsed, hc.GetTimeout())
			if err != nil {
				return infrav1.TerraformHealthCheckFailed(
					terraform,
					err.Error(),
				), err
			}
		default:
			err := fmt.Errorf("invalid health check type: %s", hc.Type)
			return infrav1.TerraformHealthCheckFailed(
				terraform,
				err.Error(),
			), err
		}
	}
	terraform = infrav1.TerraformHealthCheckSucceeded(terraform, "Health checks succeeded")
	return terraform, nil
}

func (r *TerraformReconciler) doTCPHealthCheck(ctx context.Context, name string, address string, timeout time.Duration) error {
	log := ctrl.LoggerFrom(ctx)

	// validate tcp address
	_, err := url.ParseRequestURI(address)
	if err != nil {
		return fmt.Errorf("invalid url for http health check: %s, %s", address, err)
	}

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return fmt.Errorf("failed to perform tcp health check for %s on %s: %s", name, address, err.Error())
	}

	err = conn.Close()
	if err != nil {
		log.Error(err, "Unexpected error closing TCP health check socket")
	}

	return nil
}

func (r *TerraformReconciler) doHTTPHealthCheck(ctx context.Context, name string, urlString string, timeout time.Duration) error {
	log := ctrl.LoggerFrom(ctx)

	// validate url
	_, err := url.ParseRequestURI(urlString)
	if err != nil {
		return fmt.Errorf("invalid url for http health check: %s, %s", urlString, err)
	}

	req, err := http.NewRequest("GET", urlString, nil)
	if err != nil {
		log.Error(err, "Unexpected error creating HTTP request")
		return err
	}

	ctxt, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()

	re, err := http.DefaultClient.Do(req.WithContext(ctxt))
	if err != nil {
		return fmt.Errorf("failed to perform http health check for %s on %s: %s", name, urlString, err.Error())
	}
	defer func() {
		if rerr := re.Body.Close(); rerr != nil {
			log.Error(err, "Unexpected error closing HTTP health check socket")
		}
	}()

	// read http body
	b, err := io.ReadAll(re.Body)
	if err != nil {
		return fmt.Errorf("failed to perform http health check for %s on %s, error reading body: %s", name, urlString, err.Error())
	}

	// check http status code
	if re.StatusCode >= http.StatusOK && re.StatusCode < http.StatusBadRequest {
		log.Info("HTTP health check succeeded for %s on %s, response: %v", name, urlString, *re)
		return nil
	}

	err = fmt.Errorf("failed to perform http health check for %s on %s, response body: %v", name, urlString, string(b))
	log.Error(err, "failed to perform http health check for %s on %s, response body: %v", name, urlString, string(b))
	return err
}

// parse template string from map[string]string
func (r *TerraformReconciler) templateParse(content map[string]string, text string) (string, error) {
	var b bytes.Buffer
	tmpl, err := template.New("tmpl").Parse(text)
	if err != nil {
		return "", err
	}
	err = tmpl.Execute(&b, content)
	if err != nil {
		err = fmt.Errorf("error getting terraform output for health checks: %s", err)
		return "", err
	}
	return b.String(), nil
}

// reconcileRunnerSecret reconciles the runner secret used for mtls
// it should create the secret if it doesn't exist and then verify that the cert is valid
// if the cert is not present in the secret or is invalid, it will generate a new cert and
// write it to the secret
func (r *TerraformReconciler) reconcileRunnerSecret(ctx context.Context, terraform *infrav1.Terraform) (string, error) {
	secretName := fmt.Sprintf("%s-runner.tls", terraform.Name)

	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: terraform.Namespace,
			Name:      secretName,
		},
	}

	if err := r.Get(ctx, client.ObjectKeyFromObject(tlsSecret), tlsSecret); err != nil {
		if !apierrors.IsNotFound(err) {
			return "", err
		}
		// create the runner tls secret
		if err := r.Create(ctx, tlsSecret); err != nil {
			return "", err
		}
	}

	runnerHostname := fmt.Sprintf("%s.%s.svc.cluster.local", terraform.Name, terraform.Namespace)

	if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
		runnerHostname = "localhost"
	}

	if err := r.CertRotator.RefreshRunnerCertIfNeeded(runnerHostname, tlsSecret); err != nil {
		return "", err
	}

	return secretName, nil
}

func (r *TerraformReconciler) reconcileRunnerPod(ctx context.Context, terraform *infrav1.Terraform, tlsSecretName string) error {
	runner := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: terraform.Namespace,
			Name:      fmt.Sprintf("%s-runner", terraform.Name),
		},
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(runner), runner); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		runner.Spec = runnerPodSpec(tlsSecretName)
		if err := r.Create(ctx, runner); err != nil {
			return err
		}
	}

	runnerSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: terraform.Namespace,
			Name:      fmt.Sprintf("%s-runner", terraform.Name),
		},
	}

	if err := r.Get(ctx, client.ObjectKeyFromObject(runnerSvc), runnerSvc); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		runnerSvc.Spec = corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "grpc",
					Port:       5555,
					TargetPort: intstr.FromInt(5555),
				},
			},
			Selector: map[string]string{
				"app": fmt.Sprintf("%s-runner", terraform.Name),
			},
		}
		if err := r.Create(ctx, runnerSvc); err != nil {
			return err
		}
	}

	return nil
}

func runnerPodSpec(tlsSecretName string) corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name: "grpc",
				//TODO: set correct image path
				Image: "ghcr.io/weaveworks/tf-controller/runner-server:latest",
				Ports: []corev1.ContainerPort{
					{
						Name:          "grpc",
						ContainerPort: 5555,
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "tls",
						MountPath: "/tls",
						ReadOnly:  true,
					},
				},
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: tlsSecretName,
					},
				},
			},
		},
	}
}

func (r *TerraformReconciler) getRunnerConnection(ctx context.Context, terraform *infrav1.Terraform, addr string) (*grpc.ClientConn, error) {
	tlsSecret := &corev1.Secret{}
	if err := r.Client.Get(ctx, r.CertRotator.SecretKey, tlsSecret); err != nil {
		return nil, err
	}

	creds, err := mtls.GetGRPCClientCredentials(tlsSecret)
	if err != nil {
		return nil, err
	}

	return grpc.Dial(addr, grpc.WithTransportCredentials(creds))
}
