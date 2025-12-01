package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/flux-iac/tofu-controller/api/planid"
	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/utils"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	resourceDataMaxSizeBytes = 1 * 1024 * 1024 // 1MB
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
	if err := r.writePlanAsSecret(ctx, req.Name, req.Namespace, log, planId, tfplan, "", req.Uuid); err != nil {
		return nil, err
	}

	if r.terraform.Spec.StoreReadablePlan == "json" {
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

		if err := r.writePlanAsSecret(ctx, req.Name, req.Namespace, log, planId, jsonBytes, ".json", req.Uuid); err != nil {
			return nil, err
		}

	} else if r.terraform.Spec.StoreReadablePlan == "human" {
		rawOutput, err := r.tfShowPlanFileRaw(ctx, TFPlanName)
		if err != nil {
			log.Error(err, "unable to get the plan output for human")
			return nil, err
		}

		if err := r.writePlanAsConfigMap(ctx, req.Name, req.Namespace, log, planId, rawOutput, "", req.Uuid); err != nil {
			return nil, err
		}
	}

	return &SaveTFPlanReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) generatePlanSecrets(name string, namespace string, planId string, suffix string, uuid string, plan []byte) ([]*v1.Secret, error) {
	// Build a standard name prefix for the secrets
	secretIdentifier := "tfplan-" + r.terraform.WorkspaceName() + "-" + name + suffix

	// Check whether the Terraform Plan is larger (or equal) to 1MB
	// which is the maximum size for a Kubernetes Secret or ConfigMap
	if len(plan) <= resourceDataMaxSizeBytes {
		data := map[string][]byte{TFPlanName: plan}

		// Build an individual secret containing the whole plan
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretIdentifier,
				Namespace: namespace,
				Annotations: map[string]string{
					"encoding":                "gzip",
					SavedPlanSecretAnnotation: planId,
				},
				Labels: map[string]string{
					"infra.contrib.fluxcd.io/plan-name":      name + suffix,
					"infra.contrib.fluxcd.io/plan-workspace": r.terraform.WorkspaceName(),
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: infrav1.GroupVersion.Group + "/" + infrav1.GroupVersion.Version,
						Kind:       infrav1.TerraformKind,
						Name:       name,
						UID:        types.UID(uuid),
					},
				},
			},
			Type: v1.SecretTypeOpaque,
			Data: data,
		}
		return []*v1.Secret{secret}, nil
	}

	// Otherwise, we assume that the plan needs to be "chunked"
	numChunks := (len(plan) + resourceDataMaxSizeBytes - 1) / resourceDataMaxSizeBytes

	secrets := make([]*v1.Secret, 0, numChunks)

	for chunk := range numChunks {
		start := chunk * resourceDataMaxSizeBytes
		end := min(start+resourceDataMaxSizeBytes, len(plan))

		data := map[string][]byte{TFPlanName: plan[start:end]}

		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", secretIdentifier, chunk),
				Namespace: namespace,
				Annotations: map[string]string{
					"encoding":                           "gzip",
					SavedPlanSecretAnnotation:            planId,
					"infra.contrib.fluxcd.io/plan-chunk": fmt.Sprintf("%d", chunk),
				},
				Labels: map[string]string{
					"infra.contrib.fluxcd.io/plan-name":      name + suffix,
					"infra.contrib.fluxcd.io/plan-workspace": r.terraform.WorkspaceName(),
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: infrav1.GroupVersion.Group + "/" + infrav1.GroupVersion.Version,
						Kind:       infrav1.TerraformKind,
						Name:       name,
						UID:        types.UID(uuid),
					},
				},
			},
			Type: v1.SecretTypeOpaque,
			Data: data,
		}

		secrets = append(secrets, secret)
	}

	return secrets, nil
}

func (r *TerraformRunnerServer) writePlanAsSecret(ctx context.Context, name string, namespace string, log logr.Logger, planId string, tfplan []byte, suffix string, uuid string) error {
	existingSecrets := &v1.SecretList{}

	// Try to get any secrets by using the plan labels
	// This covers "chunked" secrets as well as a single secret
	if err := r.Client.List(ctx, existingSecrets, client.InNamespace(namespace), client.MatchingLabels{
		"infra.contrib.fluxcd.io/plan-name":      name + suffix,
		"infra.contrib.fluxcd.io/plan-workspace": r.terraform.WorkspaceName(),
	}); err != nil {
		log.Error(err, "unable to list existing plan secrets")
		return err
	}

	// Check for a legacy secret if none found with the
	// new labels
	if len(existingSecrets.Items) == 0 {
		var legacyPlanSecret v1.Secret

		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "tfplan-" + r.terraform.WorkspaceName() + "-" + name + suffix}, &legacyPlanSecret); err == nil {
			existingSecrets.Items = append(existingSecrets.Items, legacyPlanSecret)
		}
	}

	// Clear up any of the old secrets first
	for _, s := range existingSecrets.Items {
		if err := r.Client.Delete(ctx, &s); err != nil {
			log.Error(err, "unable to delete existing plan secret", "secretName", s.Name)
			return err
		}
	}

	// GZIP encode the plan data
	tfplan, err := utils.GzipEncode(tfplan)
	if err != nil {
		log.Error(err, "unable to encode the plan revision", "planId", planId)
		return err
	}

	// Chunk the data if required
	secrets, err := r.generatePlanSecrets(name, namespace, planId, suffix, uuid, tfplan)
	if err != nil {
		log.Error(err, "unable to generate plan secrets", "planId", planId)
		return err
	}

	// We shouldn't have to check whether any of these already exist, as we've done
	// that above
	for _, secret := range secrets {
		// now create the seecret
		if err := r.Client.Create(ctx, secret); err != nil {
			err = fmt.Errorf("error recording plan status: %s", err)
			log.Error(err, "unable to create plan secret")
			return err
		}
	}

	return nil
}

func (r *TerraformRunnerServer) generatePlanConfigMaps(name string, namespace string, planId string, suffix string, uuid string, plan string) ([]*v1.ConfigMap, error) {
	// Build a standard name prefix for the configmaps
	configMapIdentifier := "tfplan-" + r.terraform.WorkspaceName() + "-" + name + suffix

	// Check whether the Terraform Plan is larger (or equal) to 1MB
	// which is the maximum size for a Kubernetes Secret or ConfigMap
	if len(plan) <= resourceDataMaxSizeBytes {
		data := map[string]string{TFPlanName: plan}

		// Build an individual secret containing the whole plan
		configMap := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapIdentifier,
				Namespace: namespace,
				Annotations: map[string]string{
					SavedPlanSecretAnnotation: planId,
				},
				Labels: map[string]string{
					"infra.contrib.fluxcd.io/plan-name":      name + suffix,
					"infra.contrib.fluxcd.io/plan-workspace": r.terraform.WorkspaceName(),
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: infrav1.GroupVersion.Group + "/" + infrav1.GroupVersion.Version,
						Kind:       infrav1.TerraformKind,
						Name:       name,
						UID:        types.UID(uuid),
					},
				},
			},
			Data: data,
		}
		return []*v1.ConfigMap{configMap}, nil
	}

	// Otherwise, we assume that the plan needs to be "chunked"
	numChunks := (len(plan) + resourceDataMaxSizeBytes - 1) / resourceDataMaxSizeBytes

	configMaps := make([]*v1.ConfigMap, 0, numChunks)

	for chunk := range numChunks {
		start := chunk * resourceDataMaxSizeBytes
		end := min(start+resourceDataMaxSizeBytes, len(plan))

		data := map[string]string{TFPlanName: plan[start:end]}

		configMap := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", configMapIdentifier, chunk),
				Namespace: namespace,
				Annotations: map[string]string{
					SavedPlanSecretAnnotation:            planId,
					"infra.contrib.fluxcd.io/plan-chunk": fmt.Sprintf("%d", chunk),
				},
				Labels: map[string]string{
					"infra.contrib.fluxcd.io/plan-name":      name + suffix,
					"infra.contrib.fluxcd.io/plan-workspace": r.terraform.WorkspaceName(),
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: infrav1.GroupVersion.Group + "/" + infrav1.GroupVersion.Version,
						Kind:       infrav1.TerraformKind,
						Name:       name,
						UID:        types.UID(uuid),
					},
				},
			},
			Data: data,
		}

		configMaps = append(configMaps, configMap)
	}

	return configMaps, nil
}

func (r *TerraformRunnerServer) writePlanAsConfigMap(ctx context.Context, name string, namespace string, log logr.Logger, planId string, tfplan string, suffix string, uuid string) error {
	existingConfigMaps := &v1.ConfigMapList{}

	// Try to get any ConfigMaps by using the plan labels
	// This covers "chunked" ConfigMaps as well as a single ConfigMap
	if err := r.Client.List(ctx, existingConfigMaps, client.InNamespace(namespace), client.MatchingLabels{
		"infra.contrib.fluxcd.io/plan-name":      name + suffix,
		"infra.contrib.fluxcd.io/plan-workspace": r.terraform.WorkspaceName(),
	}); err != nil {
		log.Error(err, "unable to list existing plan ConfigMaps")
		return err
	}

	// Check for a legacy configmap if none found with the
	// new labels
	if len(existingConfigMaps.Items) == 0 {
		var legacyPlanConfigMap v1.ConfigMap

		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "tfplan-" + r.terraform.WorkspaceName() + "-" + name + suffix}, &legacyPlanConfigMap); err == nil {
			existingConfigMaps.Items = append(existingConfigMaps.Items, legacyPlanConfigMap)
		}
	}

	// Clear up any of the old ConfigMaps first
	for _, s := range existingConfigMaps.Items {
		if err := r.Client.Delete(ctx, &s); err != nil {
			log.Error(err, "unable to delete existing plan ConfigMap", "configMapName", s.Name)
			return err
		}
	}

	// Chunk the data if required
	configMaps, err := r.generatePlanConfigMaps(name, namespace, planId, suffix, uuid, tfplan)
	if err != nil {
		log.Error(err, "unable to generate plan ConfigMaps", "planId", planId)
		return err
	}

	// We shouldn't have to check whether any of these already exist, as we've done
	// that above
	for _, configmap := range configMaps {
		// now create the configmap
		if err := r.Client.Create(ctx, configmap); err != nil {
			err = fmt.Errorf("error recording plan status: %s", err)
			log.Error(err, "unable to create plan ConfigMap")
			return err
		}
	}

	return nil
}
