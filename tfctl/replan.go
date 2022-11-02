package tfctl

import (
	"context"
	"fmt"
	"github.com/fluxcd/pkg/apis/meta"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"io"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

// clear plan pending
// reconcile
// check for the plan output
// print the plan output

// Replan re-plans the given Terraform resource
func (c *CLI) Replan(out io.Writer, resource string) error {
	key := types.NamespacedName{
		Name:      resource,
		Namespace: c.namespace,
	}

	if err := replan(context.TODO(), c.client, key); err != nil {
		return err
	}

	if err := requestReconciliation(context.TODO(), c.client, key); err != nil {
		return err
	}
	fmt.Fprintf(out, " Replan requested for %s/%s\n", c.namespace, resource)

	if err := wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
		terraform := &infrav1.Terraform{}
		if err := c.client.Get(context.TODO(), key, terraform); err != nil {
			return false, err
		}

		if terraform.Status.Plan.Pending != "" {
			return true, nil
		}

		return false, nil
	}); err != nil {
		return err
	}

	return c.ShowPlan(out, resource)
}

func replan(ctx context.Context, kubeClient client.Client, namespacedName types.NamespacedName) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		terraform := &infrav1.Terraform{}
		if err := kubeClient.Get(ctx, namespacedName, terraform); err != nil {
			return err
		}
		patch := client.MergeFrom(terraform.DeepCopy())
		// clear the pending plan
		apimeta.SetStatusCondition(&terraform.Status.Conditions, metav1.Condition{
			Type:    meta.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  "ReplanRequested",
			Message: "Replan requested",
		})
		// terraform.Spec.ApprovePlan = "re" + terraform.Status.Plan.Pending
		terraform.Status.Plan.Pending = ""
		terraform.Status.LastPlannedRevision = ""
		terraform.Status.LastAttemptedRevision = ""
		return kubeClient.Status().Patch(ctx, terraform, patch, client.FieldOwner("tf-controller"))
	})
}
