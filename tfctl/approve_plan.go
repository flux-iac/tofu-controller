package tfctl

import (
	"context"
	"fmt"
	"io"

	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApprovePlan approves the pending plan for a given terraform resource
func (c *CLI) ApprovePlan(out io.Writer, resource string) error {
	key := types.NamespacedName{
		Name:      resource,
		Namespace: c.namespace,
	}
	terraform := &infrav1.Terraform{}
	if err := c.client.Get(context.TODO(), key, terraform); err != nil {
		return fmt.Errorf("resource %s not found", resource)
	}

	if terraform.Status.Plan.Pending == "" {
		fmt.Fprintf(out, "no plan pending")
		return nil
	}

	return approvePlan(context.TODO(), c.client, key, terraform)
}

func approvePlan(ctx context.Context,
	kubeClient client.Client,
	namespacedName types.NamespacedName,
	terraform *infrav1.Terraform,
) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		terraform := &infrav1.Terraform{}
		if err := kubeClient.Get(ctx, namespacedName, terraform); err != nil {
			return err
		}
		patch := client.MergeFrom(terraform.DeepCopy())
		terraform.Spec.ApprovePlan = terraform.Status.Plan.Pending
		return kubeClient.Patch(ctx, terraform, patch)
	})
}
