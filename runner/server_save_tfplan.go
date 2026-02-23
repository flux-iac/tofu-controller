package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/flux-iac/tofu-controller/api/plan"
	"github.com/flux-iac/tofu-controller/api/planid"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *TerraformRunnerServer) SaveTFPlan(ctx context.Context, req *SaveTFPlanRequest) (*SaveTFPlanReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("save the plan")

	if err := r.ValidateInstanceID(req.TfInstance); err != nil {
		log.Error(err, "terraform session mismatch when saving the plan")

		return nil, err
	}

	var tfplan []byte
	if req.BackendCompletelyDisable {
		tfplan = []byte("dummy plan")
	} else {
		var err error
		tfplan, err = os.ReadFile(filepath.Join(r.tf.WorkingDir(), TFPlanName))
		if err != nil {
			err = fmt.Errorf("error reading plan file: %s", err)
			log.Error(err, "unable to complete SaveTFPlan function")
			return nil, err
		}
	}

	// planid must be the short plan id format
	planId := planid.GetPlanID(req.Revision)

	// Create the Plan object
	tfPlan, err := plan.NewFromBytes(req.Name, req.Namespace, r.terraform.WorkspaceName(), req.Uuid, planId, tfplan)
	if err != nil {
		return nil, err
	}

	if err := r.writePlanAsSecret(ctx, req.Name, req.Namespace, log, planId, tfPlan, "", req.Uuid); err != nil {
		return nil, err
	}

	switch r.terraform.Spec.StoreReadablePlan {
	case "json":
		planObj, err := r.tfShowPlanFile(ctx, TFPlanName)
		if err != nil {
			log.Error(err, "unable to get the plan output for json")
			return nil, err
		}

		jsonBytes, err := json.Marshal(planObj)
		if err != nil {
			log.Error(err, "unable to marshal the plan to json")
			return nil, err
		}

		jsonPlan, err := plan.NewFromBytes(req.Name, req.Namespace, r.terraform.WorkspaceName(), req.Uuid, planId, jsonBytes)
		if err != nil {
			log.Error(err, "Unable to create plan")
			return nil, err
		}

		if err := r.writePlanAsSecret(ctx, req.Name, req.Namespace, log, planId, jsonPlan, ".json", req.Uuid); err != nil {
			log.Error(err, "unable to write the plan to secret")
			return nil, err
		}
	case "human":
		rawOutput, err := r.tfShowPlanFileRaw(ctx, TFPlanName)
		if err != nil {
			log.Error(err, "unable to get the plan output for human")
			return nil, err
		}

		rawPlan, err := plan.NewFromBytes(req.Name, req.Namespace, r.terraform.WorkspaceName(), req.Uuid, planId, []byte(rawOutput))
		if err != nil {
			log.Error(err, "Unable to create plan")
			return nil, err
		}

		if err := r.writePlanAsConfigMap(ctx, req.Name, req.Namespace, log, planId, rawPlan, "", req.Uuid); err != nil {
			log.Error(err, "unable to write the plan to configmap")
			return nil, err
		}
	}

	return &SaveTFPlanReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) writePlanAsSecret(ctx context.Context, name string, namespace string, log logr.Logger, planId string, tfPlan *plan.Plan, suffix string, uuid string) error {
	existingSecrets := &v1.SecretList{}

	// Try to get any secrets by using the plan labels
	// This covers "chunked" secrets as well as a single secret
	if err := r.List(ctx, existingSecrets, client.InNamespace(namespace), client.MatchingLabels{
		plan.TFPlanNameLabel:      plan.SafeLabelValue(name + suffix),
		plan.TFPlanWorkspaceLabel: r.terraform.WorkspaceName(),
	}); err != nil {
		log.Error(err, "unable to list existing plan secrets")
		return err
	}

	// Check for a legacy secret if none found with the
	// new labels
	if len(existingSecrets.Items) == 0 {
		var legacyPlanSecret v1.Secret

		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "tfplan-" + r.terraform.WorkspaceName() + "-" + name + suffix}, &legacyPlanSecret); err == nil {
			existingSecrets.Items = append(existingSecrets.Items, legacyPlanSecret)
		}
	}

	// Clear up any of the old secrets first
	for _, s := range existingSecrets.Items {
		if err := r.Delete(ctx, &s); err != nil {
			log.Error(err, "unable to delete existing plan secret", "secretName", s.Name)
			return err
		}
	}

	secrets, err := tfPlan.ToSecret(suffix)
	if err != nil {
		log.Error(err, "unable to generate plan secrets", "planId", planId)
		return err
	}

	// We shouldn't have to check whether any of these already exist, as we've done
	// that above
	for _, secret := range secrets {
		// now create the seecret
		if err := r.Create(ctx, secret); err != nil {
			err = fmt.Errorf("error recording plan status: %s", err)
			log.Error(err, "unable to create plan secret")
			return err
		}
	}

	return nil
}

func (r *TerraformRunnerServer) writePlanAsConfigMap(ctx context.Context, name string, namespace string, log logr.Logger, planId string, tfPlan *plan.Plan, suffix string, uuid string) error {
	existingConfigMaps := &v1.ConfigMapList{}

	// Try to get any ConfigMaps by using the plan labels
	// This covers "chunked" ConfigMaps as well as a single ConfigMap
	if err := r.List(ctx, existingConfigMaps, client.InNamespace(namespace), client.MatchingLabels{
		plan.TFPlanNameLabel:      plan.SafeLabelValue(name + suffix),
		plan.TFPlanWorkspaceLabel: r.terraform.WorkspaceName(),
	}); err != nil {
		log.Error(err, "unable to list existing plan ConfigMaps")
		return err
	}

	// Check for a legacy configmap if none found with the
	// new labels
	if len(existingConfigMaps.Items) == 0 {
		var legacyPlanConfigMap v1.ConfigMap

		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "tfplan-" + r.terraform.WorkspaceName() + "-" + name + suffix}, &legacyPlanConfigMap); err == nil {
			existingConfigMaps.Items = append(existingConfigMaps.Items, legacyPlanConfigMap)
		}
	}

	// Clear up any of the old ConfigMaps first
	for _, s := range existingConfigMaps.Items {
		if err := r.Delete(ctx, &s); err != nil {
			log.Error(err, "unable to delete existing plan ConfigMap", "configMapName", s.Name)
			return err
		}
	}

	configMaps, err := tfPlan.ToConfigMap(suffix)
	if err != nil {
		log.Error(err, "unable to generate plan ConfigMaps", "planId", planId)
		return err
	}

	// We shouldn't have to check whether any of these already exist, as we've done
	// that above
	for _, configmap := range configMaps {
		// now create the configmap
		if err := r.Create(ctx, configmap); err != nil {
			err = fmt.Errorf("error recording plan status: %s", err)
			log.Error(err, "unable to create plan ConfigMap")
			return err
		}
	}

	return nil
}
