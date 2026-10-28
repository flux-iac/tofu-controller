package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/flux-iac/tofu-controller/api/plan"
	"github.com/flux-iac/tofu-controller/api/planid"
	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

func (r *TerraformRunnerServer) SaveTFPlan(ctx context.Context, req *SaveTFPlanRequest) (*SaveTFPlanReply, error) {
	ctrl.LoggerFrom(ctx).Info("save the plan")
	s := sessionFrom(ctx)
	workspace := s.terraform.WorkspaceName()

	var tfplan []byte
	if req.BackendCompletelyDisable {
		tfplan = []byte("dummy plan")
	} else {
		var err error
		tfplan, err = os.ReadFile(filepath.Join(s.tf.WorkingDir(), TFPlanName))
		if err != nil {
			return nil, fmt.Errorf("error reading plan file: %s", err)
		}
	}

	// planid must be the short plan id format
	planId := planid.GetPlanID(req.Revision)

	// Create the Plan object
	tfPlan, err := plan.NewFromBytes(req.Name, req.Namespace, workspace, req.Uuid, planId, tfplan)
	if err != nil {
		return nil, err
	}

	// savedObjects reports the canonical (gzipped) plan Secrets only. The
	// readable representations written below are a different artifact with a
	// different Kind and suffix; including them would inflate the chunk count
	// the controller publishes in status.artifactRefs.plan.
	saved, err := r.writePlanAsSecret(ctx, req.Name, req.Namespace, workspace, tfPlan, "")
	if err != nil {
		return nil, err
	}

	// With the backend disabled there is no plan file to show, so there is
	// no readable plan to store.
	storeReadablePlan := s.terraform.Spec.StoreReadablePlan
	if req.BackendCompletelyDisable {
		storeReadablePlan = ""
	}

	switch storeReadablePlan {
	case "json":
		planObj, err := s.tf.ShowPlanFile(ctx, TFPlanName)
		if err != nil {
			return nil, err
		}

		jsonBytes, err := json.Marshal(planObj)
		if err != nil {
			return nil, err
		}

		jsonPlan, err := plan.NewFromBytes(req.Name, req.Namespace, workspace, req.Uuid, planId, jsonBytes)
		if err != nil {
			return nil, err
		}

		if _, err := r.writePlanAsSecret(ctx, req.Name, req.Namespace, workspace, jsonPlan, ".json"); err != nil {
			return nil, err
		}
	case "human":
		rawOutput, err := s.tf.ShowPlanFileRaw(ctx, TFPlanName)
		if err != nil {
			return nil, err
		}

		rawPlan, err := plan.NewFromBytes(req.Name, req.Namespace, workspace, req.Uuid, planId, []byte(rawOutput))
		if err != nil {
			return nil, err
		}

		if err := r.writePlanAsConfigMap(ctx, req.Name, req.Namespace, workspace, rawPlan, ""); err != nil {
			return nil, err
		}
	}

	return &SaveTFPlanReply{Message: "ok", SavedObjects: saved}, nil
}

func (r *TerraformRunnerServer) writePlanAsSecret(ctx context.Context, name, namespace, workspace string, tfPlan *plan.Plan, suffix string) ([]*SavedObject, error) {
	existingSecrets := &v1.SecretList{}

	// Try to get any secrets by using the plan labels
	// This covers "chunked" secrets as well as a single secret
	if err := r.List(ctx, existingSecrets, client.InNamespace(namespace), client.MatchingLabels{
		plan.TFPlanNameLabel:      plan.SafeLabelValue(name + suffix),
		plan.TFPlanWorkspaceLabel: plan.SafeLabelValue(workspace),
	}); err != nil {
		return nil, err
	}

	// Check for a legacy secret if none found with the
	// new labels
	if len(existingSecrets.Items) == 0 {
		var legacyPlanSecret v1.Secret

		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "tfplan-" + workspace + "-" + name + suffix}, &legacyPlanSecret); err == nil {
			existingSecrets.Items = append(existingSecrets.Items, legacyPlanSecret)
		}
	}

	// Clear up any of the old secrets first
	for _, s := range existingSecrets.Items {
		if err := r.Delete(ctx, &s); err != nil {
			return nil, err
		}
	}

	secrets, err := tfPlan.ToSecret(suffix)
	if err != nil {
		return nil, err
	}

	// We shouldn't have to check whether any of these already exist, as we've done
	// that above
	saved := make([]*SavedObject, 0, len(secrets))
	for _, secret := range secrets {
		// now create the seecret
		if err := r.Create(ctx, secret); err != nil {
			return nil, fmt.Errorf("error recording plan status: %s", err)
		}
		saved = append(saved, &SavedObject{
			ApiVersion: "v1",
			Kind:       "Secret",
			Namespace:  secret.Namespace,
			Name:       secret.Name,
		})
	}

	return saved, nil
}

func (r *TerraformRunnerServer) writePlanAsConfigMap(ctx context.Context, name, namespace, workspace string, tfPlan *plan.Plan, suffix string) error {
	existingConfigMaps := &v1.ConfigMapList{}

	// Try to get any ConfigMaps by using the plan labels
	// This covers "chunked" ConfigMaps as well as a single ConfigMap
	if err := r.List(ctx, existingConfigMaps, client.InNamespace(namespace), client.MatchingLabels{
		plan.TFPlanNameLabel:      plan.SafeLabelValue(name + suffix),
		plan.TFPlanWorkspaceLabel: plan.SafeLabelValue(workspace),
	}); err != nil {
		return err
	}

	// Check for a legacy configmap if none found with the
	// new labels
	if len(existingConfigMaps.Items) == 0 {
		var legacyPlanConfigMap v1.ConfigMap

		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "tfplan-" + workspace + "-" + name + suffix}, &legacyPlanConfigMap); err == nil {
			existingConfigMaps.Items = append(existingConfigMaps.Items, legacyPlanConfigMap)
		}
	}

	// Clear up any of the old ConfigMaps first
	for _, s := range existingConfigMaps.Items {
		if err := r.Delete(ctx, &s); err != nil {
			return err
		}
	}

	configMaps, err := tfPlan.ToConfigMap(suffix)
	if err != nil {
		return err
	}

	// We shouldn't have to check whether any of these already exist, as we've done
	// that above
	for _, configmap := range configMaps {
		// now create the configmap
		if err := r.Create(ctx, configmap); err != nil {
			return fmt.Errorf("error recording plan status: %s", err)
		}
	}

	return nil
}

func (r *TerraformRunnerServer) LoadTFPlan(ctx context.Context, req *LoadTFPlanRequest) (*LoadTFPlanReply, error) {
	ctrl.LoggerFrom(ctx).Info("loading plan from secret")

	s := sessionFrom(ctx)
	fs := afero.NewOsFs()
	return loadTFPlan(ctx, req, s.terraform, s.tf.WorkingDir(), r.Client, fs)
}

// loadTFPlan loads the plan from the secret and returns the plan as a reply.
func loadTFPlan(
	ctx context.Context,
	req *LoadTFPlanRequest,
	terraform *infrav1.Terraform,
	workingDir string,
	kubeClient client.Client,
	fs afero.Fs,
) (*LoadTFPlanReply, error) {
	if !req.BackendCompletelyDisable {
		secrets := &v1.SecretList{}

		// List relevant secrets
		if err := kubeClient.List(ctx, secrets, client.InNamespace(req.Namespace), client.MatchingLabels{
			plan.TFPlanNameLabel:      plan.SafeLabelValue(req.Name),
			plan.TFPlanWorkspaceLabel: plan.SafeLabelValue(terraform.WorkspaceName()),
		}); err != nil {
			return nil, err
		}

		// Check that we actually have some secrets to read
		if len(secrets.Items) == 0 {
			return nil, fmt.Errorf("no plan secrets found for plan %s", req.PendingPlan)
		}

		pendingPlanId := req.PendingPlan

		for _, s := range secrets.Items {
			if terraform.Spec.Force {
				continue
			}

			if s.Annotations[plan.TFPlanSavedAnnotation] != pendingPlanId {
				return nil, fmt.Errorf("pending plan %s does not match secret %s (%s)", pendingPlanId, s.Name, s.Annotations[plan.TFPlanSavedAnnotation])
			}
		}

		tfPlan, err := plan.NewFromSecrets(req.Name, req.Namespace, string(terraform.GetUID()), secrets.Items)
		if err != nil {
			return nil, err
		}

		err = afero.WriteFile(fs, filepath.Join(workingDir, TFPlanName), tfPlan.Bytes(), 0644)
		if err != nil {
			return nil, fmt.Errorf("error saving plan file to disk: %s", err)
		}
	}

	return &LoadTFPlanReply{Message: "ok"}, nil
}

// FinalizeSecrets deletes the output secret, when one is requested, and the
// plan secrets. It succeeds when it deleted anything and reports NotFound
// when there was nothing to delete. A failed Kubernetes API call is returned
// as is, so the runner reports a refused call as PermissionDenied and any
// other failure as Internal.
func (r *TerraformRunnerServer) FinalizeSecrets(ctx context.Context, req *FinalizeSecretsRequest) (*FinalizeSecretsReply, error) {
	ctrl.LoggerFrom(ctx).Info("finalize the output secrets")

	deleted := false

	// The output secret is handled first, so it is deleted even when there
	// are no plan secrets.
	if req.HasSpecifiedOutputSecret {
		outputsObjectKey := types.NamespacedName{Namespace: req.Namespace, Name: req.OutputSecretName}
		var outputsSecret v1.Secret
		if err := r.Get(ctx, outputsObjectKey, &outputsSecret); err == nil {
			if err := r.Delete(ctx, &outputsSecret); err != nil {
				return nil, err
			}
			deleted = true
		} else if !apierrors.IsNotFound(err) {
			return nil, err
		}
	}

	secrets := &v1.SecretList{}

	if err := r.List(ctx, secrets, client.InNamespace(req.Namespace), client.MatchingLabels{
		plan.TFPlanNameLabel:      plan.SafeLabelValue(req.Name),
		plan.TFPlanWorkspaceLabel: plan.SafeLabelValue(req.Workspace),
	}); err != nil {
		return nil, err
	}

	if len(secrets.Items) == 0 {
		var legacyPlanSecret v1.Secret

		if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: "tfplan-" + req.Workspace + "-" + req.Name}, &legacyPlanSecret); err == nil {
			secrets.Items = append(secrets.Items, legacyPlanSecret)
		}
	}

	for _, s := range secrets.Items {
		if err := r.Delete(ctx, &s); err != nil {
			return nil, err
		}
		deleted = true
	}

	if !deleted {
		return nil, status.Error(codes.NotFound, "no existing plan secrets found to delete")
	}

	return &FinalizeSecretsReply{Message: "ok"}, nil
}
