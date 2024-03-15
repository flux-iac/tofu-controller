package tfctl

import (
	"context"
	"fmt"
	"io"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resume sets the resume field to true on the given Terraform resource.
func (c *CLI) Resume(out io.Writer, resource string) error {
	if resource == "" {
		if err := resumeAllReconciliation(context.TODO(), c.client); err != nil {
			return fmt.Errorf("failed to resume reconciliation for all Terraform resources: %w", err)
		}

		fmt.Fprint(out, " Reconciliation resumed for all Terraform resources\n")

		return nil
	}

	key := types.NamespacedName{
		Name:      resource,
		Namespace: c.namespace,
	}

	err := resumeReconciliation(context.TODO(), c.client, key)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, " Reconciliation resumed for %s/%s\n", c.namespace, resource)

	return nil
}

func resumeAllReconciliation(ctx context.Context, kubeClient client.Client) error {
	terraformList := &infrav1.TerraformList{}
	if err := kubeClient.List(ctx, terraformList); err != nil {
		return err
	}

	for _, terraform := range terraformList.Items {
		key := types.NamespacedName{
			Name:      terraform.Name,
			Namespace: terraform.Namespace,
		}

		if err := resumeReconciliation(ctx, kubeClient, key); err != nil {
			return err
		}
	}
	return nil
}

func resumeReconciliation(ctx context.Context, kubeClient client.Client,
	namespacedName types.NamespacedName) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		terraform := &infrav1.Terraform{}
		if err := kubeClient.Get(ctx, namespacedName, terraform); err != nil {
			return err
		}
		patch := client.MergeFrom(terraform.DeepCopy())
		terraform.Spec.Suspend = false
		return kubeClient.Patch(ctx, terraform, patch)
	})
}
