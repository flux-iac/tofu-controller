package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	infrav1 "github.com/chanwit/tf-controller/api/v1alpha1"
	"github.com/hashicorp/terraform-exec/tfexec"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TODO(piaras) add comment describing what this function does
func (r *TerraformReconciler) generateVarsForTF(ctx context.Context, terraform infrav1.Terraform, tf *tfexec.Terraform, revision string) (infrav1.Terraform, error) {
	vars := map[string]*apiextensionsv1.JSON{}
	if len(terraform.Spec.Vars) > 0 {
		for _, v := range terraform.Spec.Vars {
			vars[v.Name] = v.Value
		}
	}
	// varsFrom overwrite vars
	for _, vf := range terraform.Spec.VarsFrom {
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
					vars[key] = jsonEncodeBytes(val)
				}
			} else {
				for _, key := range vf.VarsKeys {
					vars[key] = jsonEncodeBytes(s.Data[key])
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
					vars[key] = jsonEncodeBytes([]byte(val))
				}
				for key, val := range cm.BinaryData {
					vars[key] = jsonEncodeBytes(val)
				}
			} else {
				for _, key := range vf.VarsKeys {
					if val, ok := cm.Data[key]; ok {
						vars[key] = jsonEncodeBytes([]byte(val))
					}
					if val, ok := cm.BinaryData[key]; ok {
						vars[key] = jsonEncodeBytes(val)
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

func jsonEncodeBytes(b []byte) *apiextensionsv1.JSON {
	return &apiextensionsv1.JSON{Raw: []byte(fmt.Sprintf(`"%s"`, b))}
}
