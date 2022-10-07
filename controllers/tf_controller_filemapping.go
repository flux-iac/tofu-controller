package controllers

import (
	"context"

	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/runner"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *TerraformReconciler) createRunnerFileMapping(ctx context.Context, terraform infrav1.Terraform) ([]*runner.FileMapping, error) {
	var runnerFileMappingList []*runner.FileMapping

	for _, fileMapping := range terraform.Spec.FileMappings {
		secret := &corev1.Secret{}
		secretLookupKey := types.NamespacedName{
			Namespace: terraform.Namespace,
			Name:      fileMapping.SecretRef.Name,
		}
		if err := r.Get(ctx, secretLookupKey, secret); err != nil {
			return runnerFileMappingList, err
		}

		runnerFileMappingList = append(runnerFileMappingList, &runner.FileMapping{
			Content:  secret.Data[fileMapping.SecretRef.Key],
			Location: fileMapping.Location,
			Path:     fileMapping.Path,
		})
	}

	return runnerFileMappingList, nil
}
