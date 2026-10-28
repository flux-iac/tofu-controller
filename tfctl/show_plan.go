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

	// Prefer Status.ArtifactRefs.Plan as the "is there a plan?" signal — it
	// is the typed status surface from newer controllers. Fall back to the
	// pre-existing string field for backward compatibility with older
	// controllers that have not yet populated ArtifactRefs.
	if !planExists(terraform) {
		fmt.Fprintln(out, "There is no plan pending.")
		return nil
	}

	planNamespace := planRefNamespace(terraform, c.namespace)

	if terraform.Spec.StoreReadablePlan == "human" {
		plan, err := readPlanFromConfigmap(ctx, c.client, resource, planNamespace, terraform.WorkspaceName(), uuid)
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
		plan, err := readPlanFromSecret(ctx, c.client, resource, planNamespace, terraform.WorkspaceName(), uuid)
		if err != nil {
			return err
		}

		fmt.Fprint(out, plan)
	}

	return nil
}

// planExists reports whether the Terraform resource has a plan to display.
// It prefers the typed Status.ArtifactRefs.Plan reference and falls back to
// the legacy Status.Plan.Pending string field for controllers that have not
// yet populated the typed refs.
func planExists(terraform *infrav1.Terraform) bool {
	if terraform.Status.ArtifactRefs != nil && terraform.Status.ArtifactRefs.Plan != nil {
		return true
	}
	return terraform.Status.Plan.Pending != ""
}

// planRefNamespace returns the namespace to scan for plan chunks. When the
// typed reference is present it is authoritative; otherwise we fall back to
// the CLI's configured namespace, matching the legacy assumption that the
// plan lives in the same namespace as the Terraform CR.
func planRefNamespace(terraform *infrav1.Terraform, fallback string) string {
	if refs := terraform.Status.ArtifactRefs; refs != nil && refs.Plan != nil {
		// Only an in-cluster locator names a namespace this CLI can read.
		if ref, ok := refs.Plan.Locator.KubernetesRef(); ok && ref.Namespace != "" {
			return ref.Namespace
		}
	}
	return fallback
}

func readPlanFromConfigmap(ctx context.Context, kubeClient client.Client, resource string, namespace string, workspace string, uuid string) (string, error) {
	configMaps := &v1.ConfigMapList{}

	// List relevant configmaps
	if err := kubeClient.List(ctx, configMaps, client.InNamespace(namespace), client.MatchingLabels{
		plan.TFPlanNameLabel:      plan.SafeLabelValue(resource),
		plan.TFPlanWorkspaceLabel: plan.SafeLabelValue(workspace),
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
		plan.TFPlanNameLabel:      plan.SafeLabelValue(resource),
		plan.TFPlanWorkspaceLabel: plan.SafeLabelValue(workspace),
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
