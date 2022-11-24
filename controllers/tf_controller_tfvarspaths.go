package controllers

import (
	securejoin "github.com/cyphar/filepath-securejoin"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
)

func (r *TerraformReconciler) getTfVarsPaths(terraform infrav1.Terraform, sourceRefRootDir string) ([]string, error) {
	var tfVarsPaths []string
	for _, path := range terraform.Spec.TfVarsPaths {
		securePath, err := securejoin.SecureJoin(sourceRefRootDir, path)
		if err != nil {
			return nil, err
		}
		tfVarsPaths = append(tfVarsPaths, securePath)
	}

	return tfVarsPaths, nil
}
