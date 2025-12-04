package tfctl

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strconv"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	v1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func gzipDecode(encodedPlan []byte) ([]byte, error) {
	re := bytes.NewReader(encodedPlan)
	gr, err := gzip.NewReader(re)
	if err != nil {
		return nil, err
	}

	o, err := io.ReadAll(gr)
	if err != nil {
		return nil, err
	}

	if err = gr.Close(); err != nil {
		return nil, err
	}
	return o, nil
}

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
		plan, err := readPlanFromConfigmap(ctx, c.client, resource, c.namespace, terraform.WorkspaceName())
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
		plan, err := readPlanFromSecret(ctx, c.client, resource, c.namespace, terraform.WorkspaceName())
		if err != nil {
			return err
		}

		fmt.Fprint(out, plan)
	}

	return nil
}

func readPlanFromConfigmap(ctx context.Context, kubeClient client.Client, resource string, namespace string, workspace string) (string, error) {
	configMaps := &v1.ConfigMapList{}

	// List relevant configmaps
	if err := kubeClient.List(ctx, configMaps, client.InNamespace(namespace), client.MatchingLabels{
		"infra.contrib.fluxcd.io/plan-name":      resource,
		"infra.contrib.fluxcd.io/plan-workspace": workspace,
	}); err != nil {
		return "", fmt.Errorf("unable to list existing plan configmaps: %s", err)
	}

	// Check that we actually have some configmaps to read
	if len(configMaps.Items) == 0 {
		return "", fmt.Errorf("no plan configmaps found for plan %s", resource)
	}

	// To store the individual plan chunks by index
	chunkMap := make(map[int]string)

	for _, configMap := range configMaps.Items {
		planStr, ok := configMap.Data["tfplan"]
		if !ok {
			return "", fmt.Errorf("configmap %s missing key tfplan", configMap.Name)
		}

		// Grab the chunk index from the configmap annotation
		chunkIndex := 0
		if idxStr, ok := configMap.Annotations["infra.contrib.fluxcd.io/plan-chunk"]; ok && idxStr != "" {
			var err error
			chunkIndex, err = strconv.Atoi(idxStr)
			if err != nil {
				return "", fmt.Errorf("invalid chunk index annotation found on configmap %s: %s", configMap.Name, err)
			}
		}

		chunkMap[chunkIndex] = planStr
	}

	var plan string

	// we know the number of chunks we "should" have, so work
	// up til there checking we have each chunk
	for i := 0; i < len(chunkMap); i++ {
		chunk, ok := chunkMap[i]
		if !ok {
			return "", fmt.Errorf("missing chunk %d for terraform %s", i, resource)
		}
		plan += chunk
	}

	return plan, nil
}

func readPlanFromSecret(ctx context.Context, kubeClient client.Client, resource string, namespace string, workspace string) (string, error) {
	secrets := &v1.SecretList{}

	// List relevant secrets
	if err := kubeClient.List(ctx, secrets, client.InNamespace(namespace), client.MatchingLabels{
		"infra.contrib.fluxcd.io/plan-name":      resource,
		"infra.contrib.fluxcd.io/plan-workspace": workspace,
	}); err != nil {
		return "", fmt.Errorf("unable to list existing plan secrets: %s", err)
	}

	// Check that we actually have some secrets to read
	if len(secrets.Items) == 0 {
		return "", fmt.Errorf("no plan secrets found for plan %s", resource)
	}

	// To store the individual plan chunks by index
	chunkMap := make(map[int][]byte)

	for _, secret := range secrets.Items {
		planStr, ok := secret.Data["tfplan"]
		if !ok {
			return "", fmt.Errorf("secret %s missing key tfplan", secret.Name)
		}

		// Grab the chunk index from the secret annotation
		chunkIndex := 0
		if idxStr, ok := secret.Annotations["infra.contrib.fluxcd.io/plan-chunk"]; ok && idxStr != "" {
			var err error
			chunkIndex, err = strconv.Atoi(idxStr)
			if err != nil {
				return "", fmt.Errorf("invalid chunk index annotation found on secret %s: %s", secret.Name, err)
			}
		}

		chunkMap[chunkIndex] = planStr
	}

	var planBytes []byte

	// we know the number of chunks we "should" have, so work
	// up til there checking we have each chunk
	for i := 0; i < len(chunkMap); i++ {
		chunk, ok := chunkMap[i]
		if !ok {
			return "", fmt.Errorf("missing chunk %d for terraform %s", i, resource)
		}
		planBytes = append(planBytes, chunk...)
	}

	data, err := gzipDecode(planBytes)
	if err != nil {
		return "", fmt.Errorf("failed to decode plan for resources %s: %s", resource, err)
	}

	return string(data), nil
}
