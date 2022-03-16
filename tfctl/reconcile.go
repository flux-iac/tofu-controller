package tfctl

import (
	"context"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconcile annotates the given object
func (c *CLI) Reconcile(resource string) error {
	key := types.NamespacedName{
		Name:      resource,
		Namespace: c.namespace,
	}

	err := requestReconciliation(context.TODO(), c.client, key)
	if err != nil {
		return err
	}

	return nil
}

func requestReconciliation(ctx context.Context, kubeClient client.Client,
	namespacedName types.NamespacedName) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
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
