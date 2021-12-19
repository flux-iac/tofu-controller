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
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	infrav1 "github.com/chanwit/tf-controller/api/v1alpha1"
	"github.com/hashicorp/terraform-exec/tfexec"
	// "github.com/fluxcd/pkg/runtime/events"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/metrics"
	"github.com/fluxcd/pkg/runtime/predicates"
	"github.com/fluxcd/pkg/untar"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-retryablehttp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/reference"
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
	httpClient      *retryablehttp.Client
	MetricsRecorder *metrics.Recorder
	Scheme          *runtime.Scheme
}

//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms/finalizers,verbs=update

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
	log := logr.FromContext(ctx)
	reconcileStart := time.Now()

	var terraform infrav1.Terraform
	if err := r.Get(ctx, req.NamespacedName, &terraform); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// resolve source reference
	source, err := r.getSource(ctx, terraform)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("Source '%s' not found", terraform.Spec.SourceRef.String())
			terraform = infrav1.TerraformNotReady(terraform, "", infrav1.ArtifactFailedReason, msg)
			if err := r.patchStatus(ctx, req, terraform.Status); err != nil {
				log.Error(err, "unable to update status for source not found")
				return ctrl.Result{Requeue: true}, err
			}
			r.recordReadiness(ctx, terraform)
			log.Info(msg)
			// do not requeue immediately, when the source is created the watcher should trigger a reconciliation
			return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
		} else {
			// retry on transient errors
			return ctrl.Result{Requeue: true}, err
		}
	}

	if source.GetArtifact() == nil {
		msg := "Source is not ready, artifact not found"
		terraform = infrav1.TerraformNotReady(terraform, "", infrav1.ArtifactFailedReason, msg)
		if err := r.patchStatus(ctx, req, terraform.Status); err != nil {
			log.Error(err, "unable to update status for artifact not found")
			return ctrl.Result{Requeue: true}, err
		}
		r.recordReadiness(ctx, terraform)
		log.Info(msg)
		// do not requeue immediately, when the artifact is created the watcher should trigger a reconciliation
		return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
	}

	// reconcile Terraform by applying the latest revision
	reconciledKustomization, reconcileErr := r.reconcile(ctx, *terraform.DeepCopy(), source)
	if err := r.patchStatus(ctx, req, reconciledKustomization.Status); err != nil {
		log.Error(err, "unable to update status after reconciliation")
		return ctrl.Result{Requeue: true}, err
	}
	r.recordReadiness(ctx, reconciledKustomization)
	// broadcast the reconciliation failure and requeue at the specified retry interval
	if reconcileErr != nil {
		log.Error(reconcileErr, fmt.Sprintf("Reconciliation failed after %s, next try in %s",
			time.Since(reconcileStart).String(),
			terraform.GetRetryInterval().String()),
			"revision",
			source.GetArtifact().Revision)
		// TODO r.event(ctx, reconciledKustomization, source.GetArtifact().Revision, events.EventSeverityError, reconcileErr.Error(), nil)
		return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
	}
	return ctrl.Result{}, nil
}

func (r *TerraformReconciler) shouldPlan(terraform infrav1.Terraform) bool {
	if terraform.Status.Plan.Pending == "" {
		return true
	} else if terraform.Status.Plan.Pending != "" {
		return false
	}
	return false
}

func (r *TerraformReconciler) shouldApply(terraform infrav1.Terraform) bool {
	if terraform.Spec.ApprovePlan == "" {
		return false
	} else if terraform.Spec.ApprovePlan == "auto" && terraform.Status.Plan.Pending != "" {
		return true
	} else if terraform.Spec.ApprovePlan == terraform.Status.Plan.Pending {
		return true
	}
	return false
}

func (r *TerraformReconciler) reconcile(
	ctx context.Context,
	terraform infrav1.Terraform,
	source sourcev1.Source) (infrav1.Terraform, error) {

	log := logr.FromContext(ctx)
	revision := source.GetArtifact().Revision

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
	err = r.download(source.GetArtifact(), tmpDir)
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

	/*
		// TODO terraform binary should be install once
		installer := &releases.ExactVersion{
			Product: product.Terraform,
			Version: version.Must(version.NewVersion("1.0.6")),
		}
		execPath, err := installer.Install(ctx)
		if err != nil {
			err = fmt.Errorf("error installing Terraform: %s", err)
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.TFExecInstallFailedReason,
				err.Error(),
			), err
		}
		log.Info("terraform installed", "execPath", execPath)
	*/

	// TODO configurable somehow by the controller
	execPath := "/tmp/terraform/terraform"

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

	log.Info("init terraform")
	// Progress to Initialized

	if r.shouldPlan(terraform) {
		terraform, err = r.plan(ctx, terraform, tf, revision)
		if err != nil {
			return terraform, err
		}
	}

	if r.shouldApply(terraform) {
		terraform, err = r.apply(ctx, terraform, tf, revision)
		if err != nil {
			return terraform, err
		}
	}

	return terraform, nil
}

func (r *TerraformReconciler) apply(ctx context.Context, terraform infrav1.Terraform, tf *tfexec.Terraform, revision string) (infrav1.Terraform, error) {
	// log := logr.FromContext(ctx)

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

	if tfplanSecret.Labels["savedPlan"] != terraform.Status.Plan.Pending {
		err = fmt.Errorf("error pending plan and plan's name in the secret are not matched: %s != %s",
			terraform.Status.Plan.Pending,
			tfplanSecret.Labels["savedPlan"])
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecApplyFailedReason,
			err.Error(),
		), err
	}

	tfplan := tfplanSecret.Data["tfplan"]
	err = ioutil.WriteFile(filepath.Join(tf.WorkingDir(), "tfplan"), tfplan, 0644)
	if err != nil {
		err = fmt.Errorf("error saving plan file to disk: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecApplyFailedReason,
			err.Error(),
		), err
	}

	if err := tf.Apply(ctx, tfexec.DirOrPlan("tfplan")); err != nil {
		err = fmt.Errorf("error running Apply: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecApplyFailedReason,
			err.Error(),
		), err
	}

	terraform = infrav1.TerraformApplied(terraform, "Terraform Apply Run Successfully")
	if err := r.Status().Update(ctx, &terraform); err != nil {
		err = fmt.Errorf("error recording apply status: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecApplyFailedReason,
			err.Error(),
		), err
	}

	outputs, err := tf.Output(ctx)
	if err != nil {
		// TODO should not be this Error
		// warning-like status is enough
		err = fmt.Errorf("error running Show: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.ArtifactFailedReason,
			err.Error(),
		), err
	}

	var availableOutputs []string
	for k := range outputs {
		availableOutputs = append(availableOutputs, k)
	}
	if len(availableOutputs) > 0 {
		sort.Strings(availableOutputs)
		terraform = infrav1.TerraformOutputAvailable(terraform, availableOutputs, "Terraform Outputs Available")
		err = r.Status().Update(ctx, &terraform)
		if err != nil {
			err = fmt.Errorf("error updating Output condition: %s", err)
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.ArtifactFailedReason,
				err.Error(),
			), err
		}
	}

	/*
		state, err := tf.Show(context.Background())
		if err != nil {
			err = fmt.Errorf("error running Show: %s", err)
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.ArtifactFailedReason,
				err.Error(),
			), err
		}
		fmt.Println(state.FormatVersion) // "0.1"
	*/

	if terraform.Spec.WriteOutputsToSecret != nil {
		wots := terraform.Spec.WriteOutputsToSecret
		data := map[string][]byte{}

		// not specified .spec.writeOutputsToSecret.outputs means export all
		if len(wots.Outputs) == 0 {
			for output, v := range outputs {
				bytes, _ := json.Marshal(v)
				data[output] = bytes
			}
		} else {
			// filter only defined output
			for _, output := range wots.Outputs {
				v := outputs[output]
				bytes, _ := json.Marshal(v)
				data[output] = bytes
			}
		}

		outputSecret := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      terraform.Spec.WriteOutputsToSecret.Name,
				Namespace: terraform.GetNamespace(),
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
			Data: data,
		}

		err = r.Client.Create(ctx, outputSecret)
		if err != nil {
			// TODO how to handle this kind of error?
			return terraform, err
		}
	}
	return terraform, nil
}

func (r *TerraformReconciler) plan(ctx context.Context, terraform infrav1.Terraform, tf *tfexec.Terraform, revision string) (infrav1.Terraform, error) {

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
			if err != nil && vf.Optional == true {
				return infrav1.TerraformNotReady(
					terraform,
					revision,
					infrav1.TFExecPlanFailedReason,
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
			if err != nil && vf.Optional == true {
				return infrav1.TerraformNotReady(
					terraform,
					revision,
					infrav1.TFExecPlanFailedReason,
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
			infrav1.TFExecPlanFailedReason,
			err.Error(),
		), err
	}
	varFilePath := filepath.Join(tf.WorkingDir(), "generated.auto.tfvars.json")
	if err := ioutil.WriteFile(varFilePath, jsonBytes, 0644); err != nil {
		err = fmt.Errorf("error generating var file: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecPlanFailedReason,
			err.Error(),
		), err
	}

	opts := []tfexec.PlanOption{tfexec.Out("tfplan")}
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

	if drifted {
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

		var planRev string
		parts := strings.SplitN(revision, "/", 2)
		if len(parts) == 2 {
			planRev = parts[0] + "-" + parts[1][0:10]
		} else {
			err = fmt.Errorf("revision is in the wrong format: %s", revision)
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.TFExecPlanFailedReason,
				err.Error(),
			), err
		}

		terraform = infrav1.TerraformPlannedWithChanges(terraform, planRev, "Terraform Plan Generated Successfully")

		tfplanData := map[string][]byte{"tfplan": tfplan}
		tfplanSecret := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tfplan-default-" + terraform.Name,
				Namespace: terraform.GetNamespace(),
				Labels: map[string]string{
					"savedPlan": terraform.Status.Plan.Pending,
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
		err = r.Client.Create(ctx, tfplanSecret)
	} else {
		terraform = infrav1.TerraformPlannedNoChanges(terraform, "Terraform Plan No Changed")
	}

	err = r.Status().Update(ctx, &terraform)
	if err != nil {
		err = fmt.Errorf("error recording plan status: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecPlanFailedReason,
			err.Error(),
		), err
	}
	return terraform, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TerraformReconciler) SetupWithManager(mgr ctrl.Manager) error {
	const (
		gitRepositoryIndexKey string = ".metadata.gitRepository"
		bucketIndexKey        string = ".metadata.bucket"
		SingleInstance               = 1
	)

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
	var source sourcev1.Source
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
				return source, err
			}
			return source, fmt.Errorf("unable to get source '%s': %w", namespacedName, err)
		}
		source = &repository
	case sourcev1.BucketKind:
		var bucket sourcev1.Bucket
		err := r.Client.Get(ctx, namespacedName, &bucket)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return source, err
			}
			return source, fmt.Errorf("unable to get source '%s': %w", namespacedName, err)
		}
		source = &bucket
	default:
		return source, fmt.Errorf("source `%s` kind '%s' not supported",
			terraform.Spec.SourceRef.Name, terraform.Spec.SourceRef.Kind)
	}
	return source, nil
}

func (r *TerraformReconciler) download(artifact *sourcev1.Artifact, tmpDir string) error {
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

func (r *TerraformReconciler) recordReadiness(ctx context.Context, kustomization infrav1.Terraform) {
	if r.MetricsRecorder == nil {
		return
	}
	log := logr.FromContext(ctx)

	objRef, err := reference.GetReference(r.Scheme, &kustomization)
	if err != nil {
		log.Error(err, "unable to record readiness metric")
		return
	}
	if rc := apimeta.FindStatusCondition(kustomization.Status.Conditions, meta.ReadyCondition); rc != nil {
		r.MetricsRecorder.RecordCondition(*objRef, *rc, !kustomization.DeletionTimestamp.IsZero())
	} else {
		r.MetricsRecorder.RecordCondition(*objRef, metav1.Condition{
			Type:   meta.ReadyCondition,
			Status: metav1.ConditionUnknown,
		}, !kustomization.DeletionTimestamp.IsZero())
	}
}

func (r *TerraformReconciler) recordSuspension(ctx context.Context, terraform infrav1.Terraform) {
	if r.MetricsRecorder == nil {
		return
	}
	log := logr.FromContext(ctx)

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

func (r *TerraformReconciler) patchStatus(ctx context.Context, req ctrl.Request, newStatus infrav1.TerraformStatus) error {
	var terraform infrav1.Terraform
	if err := r.Get(ctx, req.NamespacedName, &terraform); err != nil {
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
