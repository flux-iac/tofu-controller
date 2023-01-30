package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/utils"
	"io/ioutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"path/filepath"
	"sigs.k8s.io/controller-runtime"
	"strings"
)

func (r *TerraformRunnerServer) SaveTFPlan(ctx context.Context, req *SaveTFPlanRequest) (*SaveTFPlanReply, error) {
	log := controllerruntime.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("save the plan")
	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	var tfplan []byte
	if req.BackendCompletelyDisable {
		tfplan = []byte("dummy plan")
	} else {
		var err error
		tfplan, err = ioutil.ReadFile(filepath.Join(r.tf.WorkingDir(), TFPlanName))
		if err != nil {
			err = fmt.Errorf("error running Plan: %s", err)
			log.Error(err, "unable to run the plan")
			return nil, err
		}
	}

	planRev := strings.Replace(req.Revision, "/", "-", 1)
	planName := "plan-" + planRev
	if err := r.writePlanAsSecret(ctx, req.Name, req.Namespace, log, planName, tfplan, "", req.Uuid); err != nil {
		return nil, err
	}

	if r.terraform.Spec.StoreReadablePlan == "json" {
		planObj, err := r.tf.ShowPlanFile(ctx, TFPlanName)
		if err != nil {
			log.Error(err, "unable to get the plan output for json")
			return nil, err
		}
		jsonBytes, err := json.Marshal(planObj)
		if err != nil {
			log.Error(err, "unable to marshal the plan to json")
			return nil, err
		}

		if err := r.writePlanAsSecret(ctx, req.Name, req.Namespace, log, planName, jsonBytes, ".json", req.Uuid); err != nil {
			return nil, err
		}

	} else if r.terraform.Spec.StoreReadablePlan == "human" {
		rawOutput, err := r.tf.ShowPlanFileRaw(ctx, TFPlanName)
		if err != nil {
			log.Error(err, "unable to get the plan output for human")
			return nil, err
		}

		if err := r.writePlanAsConfigMap(ctx, req.Name, req.Namespace, log, planName, rawOutput, "", req.Uuid); err != nil {
			return nil, err
		}
	}

	return &SaveTFPlanReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) writePlanAsSecret(ctx context.Context, name string, namespace string, log logr.Logger, planName string, tfplan []byte, suffix string, uuid string) error {
	secretName := "tfplan-" + r.terraform.WorkspaceName() + "-" + name + suffix
	tfplanObjectKey := types.NamespacedName{Name: secretName, Namespace: namespace}
	var tfplanSecret v1.Secret
	tfplanSecretExists := true

	if err := r.Client.Get(ctx, tfplanObjectKey, &tfplanSecret); err != nil {
		if errors.IsNotFound(err) {
			tfplanSecretExists = false
		} else {
			err = fmt.Errorf("error getting tfplanSecret: %s", err)
			log.Error(err, "unable to get the plan secret")
			return err
		}
	}

	if tfplanSecretExists {
		if err := r.Client.Delete(ctx, &tfplanSecret); err != nil {
			err = fmt.Errorf("error deleting tfplanSecret: %s", err)
			log.Error(err, "unable to delete the plan secret")
			return err
		}
	}

	tfplan, err := utils.GzipEncode(tfplan)
	if err != nil {
		log.Error(err, "unable to encode the plan revision", "planName", planName)
		return err
	}

	tfplanData := map[string][]byte{TFPlanName: tfplan}
	tfplanSecret = v1.Secret{
		TypeMeta: v12.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: v12.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Annotations: map[string]string{
				"encoding":                "gzip",
				SavedPlanSecretAnnotation: planName,
			},
			OwnerReferences: []v12.OwnerReference{
				{
					APIVersion: v1alpha1.GroupVersion.Group + "/" + v1alpha1.GroupVersion.Version,
					Kind:       v1alpha1.TerraformKind,
					Name:       name,
					UID:        types.UID(uuid),
				},
			},
		},
		Type: v1.SecretTypeOpaque,
		Data: tfplanData,
	}

	if err := r.Client.Create(ctx, &tfplanSecret); err != nil {
		err = fmt.Errorf("error recording plan status: %s", err)
		log.Error(err, "unable to create plan secret")
		return err
	}

	return nil
}

func (r *TerraformRunnerServer) writePlanAsConfigMap(ctx context.Context, name string, namespace string, log logr.Logger, planName string, tfplan string, suffix string, uuid string) error {
	configMapName := "tfplan-" + r.terraform.WorkspaceName() + "-" + name + suffix
	tfplanObjectKey := types.NamespacedName{Name: configMapName, Namespace: namespace}
	var tfplanCM v1.ConfigMap
	tfplanCMExists := true

	if err := r.Client.Get(ctx, tfplanObjectKey, &tfplanCM); err != nil {
		if errors.IsNotFound(err) {
			tfplanCMExists = false
		} else {
			err = fmt.Errorf("error getting tfplanSecret: %s", err)
			log.Error(err, "unable to get the plan configmap")
			return err
		}
	}

	if tfplanCMExists {
		if err := r.Client.Delete(ctx, &tfplanCM); err != nil {
			err = fmt.Errorf("error deleting tfplanSecret: %s", err)
			log.Error(err, "unable to delete the plan configmap")
			return err
		}
	}

	tfplanData := map[string]string{TFPlanName: tfplan}
	tfplanCM = v1.ConfigMap{
		TypeMeta: v12.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: v12.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Annotations: map[string]string{
				SavedPlanSecretAnnotation: planName,
			},
			OwnerReferences: []v12.OwnerReference{
				{
					APIVersion: v1alpha1.GroupVersion.Group + "/" + v1alpha1.GroupVersion.Version,
					Kind:       v1alpha1.TerraformKind,
					Name:       name,
					UID:        types.UID(uuid),
				},
			},
		},
		Data: tfplanData,
	}

	if err := r.Client.Create(ctx, &tfplanCM); err != nil {
		err = fmt.Errorf("error recording plan status: %s", err)
		log.Error(err, "unable to create plan configmap")
		return err
	}

	return nil
}
