package tfctl

import (
	"context"
	"fmt"
	"io"

	"github.com/flux-iac/tofu-controller/api/plan"
	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	v1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ShowPlan displays the plan for the given Terraform resource
func (c *CLI) ShowPlan(ctx context.Context, out io.Writer, resource string) error {
	key := types.NamespacedName{
		Name:      resource,
		Namespace: c.namespace,
	}

	terraform := &infrav1.Terraform{}
	if err := c.client.Get(ctx, key, terraform); err != nil {
		return fmt.Errorf("resource %s not found", resource)
	}

	uuid := string(terraform.GetUID())

	if terraform.Spec.StoreReadablePlan == "" || terraform.Spec.StoreReadablePlan == "none" {
		fmt.Fprintln(out, "no readable plan available")
		fmt.Fprintln(out, "please set spec.storeReadablePlan to either 'human' or 'json'")
		return nil
	}

	if terraform.Status.Plan.Pending == "" {
		fmt.Fprintln(out, "There is no plan pending.")
		return nil
	}

	if terraform.Spec.StoreReadablePlan == "human" {
		plan, err := readPlanFromConfigmap(ctx, c.client, resource, c.namespace, terraform.WorkspaceName(), uuid)
		if err != nil {
			return err
		}

		fmt.Fprintln(out, plan)

		cond := apimeta.FindStatusCondition(terraform.Status.Conditions, meta.ReadyCondition)
		if cond != nil {
			fmt.Fprintln(out, cond.Message)
			if cond.Message == "Plan generated: This object is in the plan only mode." {
				// do nothing
			} else {
				fmt.Fprintf(out, "To set the field, you can also run:\n\n  tfctl approve %s -f filename.yaml \n", resource)
			}
		}

	} else if terraform.Spec.StoreReadablePlan == "json" {
		plan, err := readPlanFromSecret(ctx, c.client, resource, c.namespace, terraform.WorkspaceName(), uuid)
		if err != nil {
			return err
		}

		fmt.Fprint(out, plan)
	}

	return nil
}

func readPlanFromConfigmap(ctx context.Context, kubeClient client.Client, resource string, namespace string, workspace string, uuid string) (string, error) {
	configMaps := &v1.ConfigMapList{}

	// List relevant configmaps
	if err := kubeClient.List(ctx, configMaps, client.InNamespace(namespace), client.MatchingLabels{
		plan.TFPlanNameLabel:      resource,
		plan.TFPlanWorkspaceLabel: workspace,
	}); err != nil {
		return "", fmt.Errorf("unable to list existing plan configmaps: %s", err)
	}

	// Check that we actually have some configmaps to read
	if len(configMaps.Items) == 0 {
		return "", fmt.Errorf("no plan configmaps found for plan %s", resource)
	}

	tfPlan, err := plan.NewFromConfigMaps(resource, namespace, uuid, configMaps.Items)
	if err != nil {
		return "", err
	}

	return tfPlan.ToString(), nil
}

func readPlanFromSecret(ctx context.Context, kubeClient client.Client, resource string, namespace string, workspace string, uuid string) (string, error) {
	secrets := &v1.SecretList{}

	// List relevant secrets
	if err := kubeClient.List(ctx, secrets, client.InNamespace(namespace), client.MatchingLabels{
		plan.TFPlanNameLabel:      resource,
		plan.TFPlanWorkspaceLabel: workspace,
	}); err != nil {
		return "", fmt.Errorf("unable to list existing plan secrets: %s", err)
	}

	// Check that we actually have some secrets to read
	if len(secrets.Items) == 0 {
		return "", fmt.Errorf("no plan secrets found for plan %s", resource)
	}

	tfPlan, err := plan.NewFromSecrets(resource, namespace, uuid, secrets.Items)
	if err != nil {
		return "", err
	}

	return tfPlan.ToString(), nil
}
