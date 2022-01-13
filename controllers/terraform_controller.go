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
	"compress/gzip"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/chanwit/tf-controller/api/v1alpha1"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/events"
	"github.com/fluxcd/pkg/runtime/metrics"
	"github.com/fluxcd/pkg/runtime/predicates"
	"github.com/fluxcd/pkg/untar"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kuberecorder "k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
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

	// Examine if the object is under deletion
	if !terraform.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, terraform)
	}

	// Return early if the Terraform is suspended.
	if terraform.Spec.Suspend {
		log.Info("Reconciliation is suspended for this object")
		return ctrl.Result{}, nil
	}

	// Return early if it's manually mode and pending
	if terraform.Status.Plan.Pending != "" && !r.forceOrAutoApply(terraform) && !r.shouldApply(terraform) {
		log.Info("Reconciliation is stopped to wait for a manual approve")
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

	// reconcile Terraform by applying the latest revision
	reconciledTerraform, reconcileErr := r.reconcile(ctx, *terraform.DeepCopy(), sourceObj)
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
	return terraform.Spec.Force || terraform.Spec.ApprovePlan == "auto"
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
	} else if terraform.Spec.ApprovePlan == "auto" && terraform.Status.Plan.Pending != "" {
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

type LocalPrintfer struct {
	logger logr.Logger
}

func (l LocalPrintfer) Printf(format string, v ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, v...))
}

func (r *TerraformReconciler) reconcile(ctx context.Context, terraform infrav1.Terraform, sourceObj sourcev1.Source) (infrav1.Terraform, error) {

	log := ctrl.LoggerFrom(ctx)
	revision := sourceObj.GetArtifact().Revision
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	terraform = infrav1.TerraformProgressing(terraform, "Initializing")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "unable to update status before Terraform initialization")
		return terraform, err
	}

	// create tmp dir
	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("%s-%s-", terraform.Namespace, terraform.Name))
	if err != nil {
		err = fmt.Errorf("tmp dir error: %w", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			sourcev1.StorageOperationFailedReason,
			err.Error(),
		), err
	}
	defer os.RemoveAll(tmpDir)

	log.Info("tmpDir created", "tmpDir", tmpDir)

	// download artifact and extract files
	err = r.downloadAndExtract(sourceObj.GetArtifact(), tmpDir)
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.ArtifactFailedReason,
			err.Error(),
		), err
	}
	log.Info("artifact downloaded")

	// check build path exists
	dirPath, err := securejoin.SecureJoin(tmpDir, terraform.Spec.Path)
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.ArtifactFailedReason,
			err.Error(),
		), err
	}

	if _, err := os.Stat(dirPath); err != nil {
		err = fmt.Errorf("terraform path not found: %w", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.ArtifactFailedReason,
			err.Error(),
		), err
	}

	const backendConfigPath = "generated_backend_config.tf"
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

	filePath, err := securejoin.SecureJoin(dirPath, backendConfigPath)
	if err != nil {
		return terraform, err
	}
	err = os.WriteFile(filePath, []byte(backendConfig), 0644)
	if err != nil {
		return terraform, err
	}

	// TODO configurable somehow by the controller
	execPath, err := exec.LookPath("terraform")
	if err != nil {
		err = fmt.Errorf("cannot find Terraform binary: %s in %s", err, os.Getenv("PATH"))
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecNewFailedReason,
			err.Error(),
		), err
	}

	workingDir := dirPath
	tf, err := tfexec.NewTerraform(workingDir, execPath)
	if err != nil {
		err = fmt.Errorf("error running NewTerraform: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecNewFailedReason,
			err.Error(),
		), err
	}
	tf.SetStdout(os.Stdout)
	tf.SetStderr(os.Stderr)
	tf.SetLogger(&LocalPrintfer{logger: log})

	log.Info("new terraform", "workingDir", workingDir)

	err = tf.Init(ctx, tfexec.Upgrade(true))
	if err != nil {
		err = fmt.Errorf("error running Init: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecInitFailedReason,
			err.Error(),
		), err
	}

	log.Info("tfexec initialized terraform")

	if terraform, err := r.generateVarsForTF(ctx, terraform, tf, revision); err != nil {
		return terraform, err
	}

	log.Info("generated var files from spec")

	if r.shouldDetectDrift(terraform, revision) {

		terraform, driftDetectionErr := r.detectDrift(ctx, terraform, tf, revision)

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

	if r.shouldPlan(terraform) {
		terraform, err = r.plan(ctx, terraform, tf, revision)
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
		terraform, err = r.apply(ctx, terraform, tf, revision, &outputs)
		if err != nil {
			return terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after applying")
			return terraform, err
		}
	}

	if r.shouldWriteOutputs(terraform, outputs) {
		terraform, err = r.writeOutput(ctx, terraform, outputs, revision)
		if err != nil {
			return terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after writing outputs")
			return terraform, err
		}
	}

	return terraform, nil
}

func (r *TerraformReconciler) detectDrift(ctx context.Context, terraform infrav1.Terraform, tf *tfexec.Terraform, revision string) (infrav1.Terraform, error) {
	const (
		driftFilename = "tfdrift"
	)

	drifted, err := tf.Plan(ctx, tfexec.Out(driftFilename), tfexec.Refresh(true))
	if err != nil {
		err = fmt.Errorf("error running Plan: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.DriftDetectionFailedReason,
			err.Error(),
		), err
	}

	if drifted {
		rawOutput, err := tf.ShowPlanFileRaw(ctx, driftFilename)
		if err != nil {
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.DriftDetectionFailedReason,
				err.Error(),
			), err
		}

		// If drift detected & we use the auto mode, then we continue
		terraform = infrav1.TerraformDriftDetected(terraform, revision, infrav1.DriftDetectedReason, rawOutput)
		return terraform, fmt.Errorf(infrav1.DriftDetectedReason)
	}

	terraform = infrav1.TerraformNoDrift(terraform, revision, infrav1.NoDriftReason, "No drift")
	return terraform, nil
}

func (r *TerraformReconciler) plan(ctx context.Context, terraform infrav1.Terraform, tf *tfexec.Terraform, revision string) (infrav1.Terraform, error) {

	log := ctrl.LoggerFrom(ctx)
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	terraform = infrav1.TerraformProgressing(terraform, "Terraform Planning")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "unable to update status before Terraform planning")
		return terraform, err
	}

	opts := []tfexec.PlanOption{tfexec.Out("tfplan")}
	if terraform.Spec.Destroy {
		opts = append(opts, tfexec.Destroy(true))
	}

	drifted, err := tf.Plan(ctx, opts...)
	if err != nil {
		err = fmt.Errorf("error running Plan: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecPlanFailedReason,
			err.Error(),
		), err
	}

	tfplan, err := ioutil.ReadFile(filepath.Join(tf.WorkingDir(), "tfplan"))
	if err != nil {
		err = fmt.Errorf("error running Plan: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecPlanFailedReason,
			err.Error(),
		), err
	}

	tfplanObjectKey := types.NamespacedName{Name: "tfplan-default-" + terraform.Name, Namespace: terraform.GetNamespace()}
	var tfplanSecret corev1.Secret
	tfplanSecretExists := true
	if err := r.Client.Get(ctx, tfplanObjectKey, &tfplanSecret); err != nil {
		if errors.IsNotFound(err) {
			tfplanSecretExists = false
		} else {
			err = fmt.Errorf("error getting tfplanSecret: %s", err)
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.TFExecPlanFailedReason,
				err.Error(),
			), err
		}
	}

	if tfplanSecretExists {
		if err := r.Client.Delete(ctx, &tfplanSecret); err != nil {
			err = fmt.Errorf("error deleting tfplanSecret: %s", err)
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.TFExecPlanFailedReason,
				err.Error(),
			), err
		}
	}

	planRev := strings.Replace(revision, "/", "-", 1)
	planName := "plan-" + planRev

	tfplanData := map[string][]byte{"tfplan": tfplan}
	tfplanSecret = corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tfplan-default-" + terraform.Name,
			Namespace: terraform.GetNamespace(),
			Labels: map[string]string{
				"savedPlan": planName,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: terraform.APIVersion,
					Kind:       terraform.Kind,
					Name:       terraform.GetName(),
					UID:        terraform.GetUID(),
				},
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: tfplanData,
	}

	if err := r.Client.Create(ctx, &tfplanSecret); err != nil {
		err = fmt.Errorf("error recording plan status: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecPlanFailedReason,
			err.Error(),
		), err
	}

	if drifted {
		terraform = infrav1.TerraformPlannedWithChanges(terraform, revision, "Plan generated")
	} else {
		terraform = infrav1.TerraformPlannedNoChanges(terraform, revision, "Plan no changes")
	}

	return terraform, nil
}

func (r *TerraformReconciler) generateVarsForTF(ctx context.Context, terraform infrav1.Terraform, tf *tfexec.Terraform, revision string) (infrav1.Terraform, error) {
	vars := map[string]string{}
	if len(terraform.Spec.Vars) > 0 {
		for _, v := range terraform.Spec.Vars {
			vars[v.Name] = v.Value
		}
	}
	// varsFrom overwrite vars
	if terraform.Spec.VarsFrom != nil {
		vf := terraform.Spec.VarsFrom
		objectKey := types.NamespacedName{
			Namespace: terraform.Namespace,
			Name:      vf.Name,
		}
		if vf.Kind == "Secret" {
			var s corev1.Secret
			err := r.Get(ctx, objectKey, &s)
			if err != nil && vf.Optional == false {
				return infrav1.TerraformNotReady(
					terraform,
					revision,
					infrav1.VarsGenerationFailedReason,
					err.Error(),
				), err
			}
			// if VarsKeys is null, use all
			if vf.VarsKeys == nil {
				for key, val := range s.Data {
					vars[key] = string(val)
				}
			} else {
				for _, key := range vf.VarsKeys {
					vars[key] = string(s.Data[key])
				}
			}
		} else if vf.Kind == "ConfigMap" {
			var cm corev1.ConfigMap
			err := r.Get(ctx, objectKey, &cm)
			if err != nil && vf.Optional == false {
				return infrav1.TerraformNotReady(
					terraform,
					revision,
					infrav1.VarsGenerationFailedReason,
					err.Error(),
				), err
			}
			// if VarsKeys is null, use all
			if vf.VarsKeys == nil {
				for key, val := range cm.Data {
					vars[key] = val
				}
				for key, val := range cm.BinaryData {
					vars[key] = string(val)
				}
			} else {
				for _, key := range vf.VarsKeys {
					if val, ok := cm.Data[key]; ok {
						vars[key] = val
					}
					if val, ok := cm.BinaryData[key]; ok {
						vars[key] = string(val)
					}
				}
			}
		}
	}

	jsonBytes, err := json.Marshal(vars)
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.VarsGenerationFailedReason,
			err.Error(),
		), err
	}

	varFilePath := filepath.Join(tf.WorkingDir(), "generated.auto.tfvars.json")
	if err := ioutil.WriteFile(varFilePath, jsonBytes, 0644); err != nil {
		err = fmt.Errorf("error generating var file: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.VarsGenerationFailedReason,
			err.Error(),
		), err
	}

	return terraform, nil
}

func (r *TerraformReconciler) apply(ctx context.Context, terraform infrav1.Terraform, tf *tfexec.Terraform, revision string, outputs *map[string]tfexec.OutputMeta) (infrav1.Terraform, error) {

	const (
		TFPlanName           = "tfplan"
		SavedPlanSecretLabel = "savedPlan"
	)

	log := ctrl.LoggerFrom(ctx)
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	terraform = infrav1.TerraformProgressing(terraform, "Applying")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "unable to update status before Terraform applying")
		return terraform, err
	}

	tfplanSecretKey := types.NamespacedName{Namespace: terraform.Namespace, Name: "tfplan-default-" + terraform.Name}
	tfplanSecret := corev1.Secret{}
	err := r.Get(ctx, tfplanSecretKey, &tfplanSecret)
	if err != nil {
		err = fmt.Errorf("error getting plan secret: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecApplyFailedReason,
			err.Error(),
		), err
	}

	if tfplanSecret.Labels[SavedPlanSecretLabel] != terraform.Status.Plan.Pending {
		err = fmt.Errorf("error pending plan and plan's name in the secret are not matched: %s != %s",
			terraform.Status.Plan.Pending,
			tfplanSecret.Labels[SavedPlanSecretLabel])
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecApplyFailedReason,
			err.Error(),
		), err
	}

	tfplan := tfplanSecret.Data[TFPlanName]
	err = ioutil.WriteFile(filepath.Join(tf.WorkingDir(), TFPlanName), tfplan, 0644)
	if err != nil {
		err = fmt.Errorf("error saving plan file to disk: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecApplyFailedReason,
			err.Error(),
		), err
	}

	terraform = infrav1.TerraformApplying(terraform, revision, "Apply started")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "error recording apply status: %s", err)
		return terraform, err
	}

	if err := tf.Apply(ctx, tfexec.DirOrPlan(TFPlanName)); err != nil {
		err = fmt.Errorf("error running Apply: %s", err)
		return infrav1.TerraformAppliedFailResetPlanAndNotReady(
			terraform,
			revision,
			infrav1.TFExecApplyFailedReason,
			err.Error(),
		), err
	}

	terraform = infrav1.TerraformApplied(terraform, revision, "Applied successfully")

	*outputs, err = tf.Output(ctx)
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

func (r *TerraformReconciler) writeOutput(ctx context.Context, terraform infrav1.Terraform, outputs map[string]tfexec.OutputMeta, revision string) (infrav1.Terraform, error) {

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
			if ct == cty.String {
				cv, err := ctyjson.Unmarshal(v.Value, ct)
				if err != nil {
					return terraform, err
				}
				data[output] = []byte(cv.AsString())
			} else {
				outputBytes, err := json.Marshal(v)
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
			// if it's a string, we can embed it directly into Secret's data
			if ct == cty.String {
				cv, err := ctyjson.Unmarshal(v.Value, ct)
				if err != nil {
					return terraform, err
				}
				data[output] = []byte(cv.AsString())
			} else {
				outputBytes, err := json.Marshal(v)
				if err != nil {
					return terraform, err
				}
				data[output] = outputBytes
			}
		}
	}

	objectKey := types.NamespacedName{Namespace: terraform.GetNamespace(), Name: terraform.Spec.WriteOutputsToSecret.Name}
	var outputSecret corev1.Secret

	if err := r.Client.Get(ctx, objectKey, &outputSecret); err == nil {
		if err := r.Client.Delete(ctx, &outputSecret); err != nil {
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.OutputsWritingFailedReason,
				err.Error(),
			), err
		}
	} else if apierrors.IsNotFound(err) == false {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.OutputsWritingFailedReason,
			err.Error(),
		), err
	}

	if len(data) == 0 || terraform.Spec.Destroy == true {
		return infrav1.TerraformOutputsWritten(terraform, revision, "No Outputs written"), nil
	} else {
		block := true
		outputSecret = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      terraform.Spec.WriteOutputsToSecret.Name,
				Namespace: terraform.GetNamespace(),
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         terraform.APIVersion,
						Kind:               terraform.Kind,
						Name:               terraform.GetName(),
						UID:                terraform.GetUID(),
						BlockOwnerDeletion: &block,
					},
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: data,
		}

		err := r.Client.Create(ctx, &outputSecret)
		if err != nil {
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.OutputsWritingFailedReason,
				err.Error(),
			), err
		}
		return infrav1.TerraformOutputsWritten(terraform, revision, "Outputs written"), nil
	}

}

// SetupWithManager sets up the controller with the Manager.
func (r *TerraformReconciler) SetupWithManager(mgr ctrl.Manager) error {
	const (
		gitRepositoryIndexKey string = ".metadata.gitRepository"
		bucketIndexKey        string = ".metadata.bucket"
		SingleInstance               = 1
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
	httpClient.RetryMax = 10 // TODO opts.HTTPRetry
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
		// SingleInstance makes it serialization
		WithOptions(controller.Options{MaxConcurrentReconciles: SingleInstance}).
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

func (r *TerraformReconciler) finalize(ctx context.Context, terraform infrav1.Terraform) (ctrl.Result, error) {
	planObjectKey := types.NamespacedName{Namespace: terraform.GetNamespace(), Name: "tfplan-default-" + terraform.Name}
	var planSecret corev1.Secret
	if err := r.Client.Get(ctx, planObjectKey, &planSecret); err == nil {
		if err := r.Client.Delete(ctx, &planSecret); err != nil {
			// transient failure
			return ctrl.Result{Requeue: true}, err
		}
	} else if apierrors.IsNotFound(err) {
		// it's ok. ignored
	} else {
		// transient failure
		return ctrl.Result{Requeue: true}, err
	}

	if terraform.Spec.WriteOutputsToSecret != nil && terraform.Spec.WriteOutputsToSecret.Name != "" {
		outputsObjectKey := types.NamespacedName{Namespace: terraform.GetNamespace(), Name: terraform.Spec.WriteOutputsToSecret.Name}
		var outputsSecret corev1.Secret
		if err := r.Client.Get(ctx, outputsObjectKey, &outputsSecret); err == nil {
			if err := r.Client.Delete(ctx, &outputsSecret); err != nil {
				// transient failure
				return ctrl.Result{Requeue: true}, err
			}
		} else if apierrors.IsNotFound(err) {
			// it's ok. ignored
		} else {
			// transient failure
			return ctrl.Result{Requeue: true}, err
		}
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

func (r *TerraformReconciler) encodePlan(terraform infrav1.Terraform, tfplan []byte) ([]byte, error) {
	e := terraform.Annotations["encoding"]

	switch e {
	case "gzip":
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)

		_, err := w.Write(tfplan)
		if err != nil {
			return nil, err
		}

		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	default:
		return nil, fmt.Errorf("%q encoding method is not valid or supported", e)
	}
}

func (r *TerraformReconciler) decodePlan(terraform infrav1.Terraform, encodedPlan []byte) ([]byte, error) {
	e := terraform.Annotations["encoding"]

	switch e {
	case "gzip":
		r := bytes.NewReader(encodedPlan)
		gr, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}

		o, err := ioutil.ReadAll(gr)
		if err != nil {
			return nil, err
		}

		if err = gr.Close(); err != nil {
			return nil, err
		}
		return o, nil
	default:
		return nil, fmt.Errorf("%q encoding method is not valid or supported", e)
	}
}
