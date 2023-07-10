package tfctl

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/fluxcd/pkg/apis/meta"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
)

func gzipDecode(encodedPlan []byte) ([]byte, error) {
	re := bytes.NewReader(encodedPlan)
	gr, err := gzip.NewReader(re)
	if err != nil {
		return nil, err
	}

	o, err := ioutil.ReadAll(gr)
	if err != nil {
		return nil, err
	}

	if err = gr.Close(); err != nil {
		return nil, err
	}
	return o, nil
}

// ShowPlan displays the plan for the given Terraform resource
func (c *CLI) ShowPlan(out io.Writer, resource string) error {
	key := types.NamespacedName{
		Name:      resource,
		Namespace: c.namespace,
	}
	terraform := &infrav1.Terraform{}
	if err := c.client.Get(context.TODO(), key, terraform); err != nil {
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
		planKey := types.NamespacedName{
			Name:      fmt.Sprintf("tfplan-%s-%s", terraform.WorkspaceName(), resource),
			Namespace: c.namespace,
		}
		var tfplanCM corev1.ConfigMap
		if err := c.client.Get(context.TODO(), planKey, &tfplanCM); err != nil {
			return fmt.Errorf("plan %s not found", planKey)
		}
		fmt.Fprintln(out, tfplanCM.Data["tfplan"])

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
		planKey := types.NamespacedName{
			Name:      fmt.Sprintf("tfplan-%s-%s.json", terraform.WorkspaceName(), resource),
			Namespace: c.namespace,
		}
		planSecret := corev1.Secret{}
		if err := c.client.Get(context.TODO(), planKey, &planSecret); err != nil {
			return fmt.Errorf("plan for resource %s not found", resource)
		}

		data, err := gzipDecode(planSecret.Data["tfplan"])
		if err != nil {
			return fmt.Errorf("failed to decode plan for resources %s: %s", resource, err)
		}
		fmt.Fprint(out, string(data))
	}

	return nil
}
