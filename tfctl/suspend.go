package tfctl

import (
	"context"

	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Suspend sets the suspend field to true on the given Terraform resource.
func (c *CLI) Suspend(resource string) error {
	key := types.NamespacedName{
		Name:      resource,
		Namespace: c.namespace,
	}

	err := suspendReconciliation(context.TODO(), c.client, key)
	if err != nil {
		return err
	}

	return nil
}

func suspendReconciliation(ctx context.Context, kubeClient client.Client,
	namespacedName types.NamespacedName) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		terraform := &infrav1.Terraform{}
		if err := kubeClient.Get(ctx, namespacedName, terraform); err != nil {
			return err
		}
		patch := client.MergeFrom(terraform.DeepCopy())
		terraform.Spec.Suspend = true
		return kubeClient.Patch(ctx, terraform, patch)
	})
}
