package controllers

import (
	"fmt"

	tfv1alpha2 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func IndexBy(kind string) func(o client.Object) []string {
	return func(o client.Object) []string {
		terraform, ok := o.(*tfv1alpha2.Terraform)
		if !ok {
			panic(fmt.Sprintf("Expected a Terraform object, got %T", o))
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
