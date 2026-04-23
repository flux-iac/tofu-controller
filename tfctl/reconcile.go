package tfctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconcile triggers a reconciliation of Terraform resources.
// If resource == "", it reconciles all resources in the namespace.
func (c *CLI) Reconcile(out io.Writer, resource string) error {
	if resource == "" {
		return reconcileAllResources(context.TODO(), out, c.client, c.namespace)
	}
	key := types.NamespacedName{
		Name:      resource,
		Namespace: c.namespace,
	}
	err := requestReconciliation(context.TODO(), c.client, key)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, " Reconcile requested for %s/%s\n", c.namespace, resource)
	return nil
}

func reconcileAllResources(ctx context.Context, out io.Writer, kubeClient client.Client, namespace string) error {
	terraformList := &infrav1.TerraformList{}
	if err := kubeClient.List(ctx, terraformList, client.InNamespace(namespace)); err != nil {
		return err
	}

	var errs []error
	for _, terraform := range terraformList.Items {
		key := types.NamespacedName{
			Name:      terraform.Name,
			Namespace: terraform.Namespace,
		}
		if err := requestReconciliation(ctx, kubeClient, key); err != nil {
			errs = append(errs, fmt.Errorf("failed to reconcile %s/%s: %w", terraform.Namespace, terraform.Name, err))
		} else {
			fmt.Fprintf(out, " Reconcile requested for %s/%s\n", terraform.Namespace, terraform.Name)
		}
	}
	return errors.Join(errs...)
}

func requestReconciliation(ctx context.Context, kubeClient client.Client, namespacedName types.NamespacedName) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		terraform := &infrav1.Terraform{}
		if err := kubeClient.Get(ctx, namespacedName, terraform); err != nil {
			return err
		}
		patch := client.MergeFrom(terraform.DeepCopy())
		if ann := terraform.GetAnnotations(); ann == nil {
			terraform.SetAnnotations(map[string]string{
				meta.ReconcileRequestAnnotation: time.Now().Format(time.RFC3339Nano),
			})
		} else {
			ann[meta.ReconcileRequestAnnotation] = time.Now().Format(time.RFC3339Nano)
			terraform.SetAnnotations(ann)
		}
		return kubeClient.Patch(ctx, terraform, patch)
	})
}
