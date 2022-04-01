package tfctl

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/hashicorp/terraform-exec/tfexec"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

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

	if terraform.Status.Plan.Pending == "" {
		fmt.Fprintln(out, "There is no plan pending.")
		return nil
	}

	planKey := types.NamespacedName{
		Name:      fmt.Sprintf("tfplan-default-%s", resource),
		Namespace: c.namespace,
	}

	planSecret := &corev1.Secret{}
	if err := c.client.Get(context.TODO(), planKey, planSecret); err != nil {
		return fmt.Errorf("plan for resource %s not found", resource)
	}

	data, err := utils.GzipDecode(planSecret.Data["tfplan"])
	if err != nil {
		return fmt.Errorf("failed to decode plan for resources %s: %s", resource, err)
	}

	tmpDir, err := ioutil.TempDir("", "tfctl")
	if err != nil {
		return err
	}

	planFile, err := ioutil.TempFile(tmpDir, "tfctl-plan-")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			fmt.Fprintf(out, "failed to remove temporary directory %s: %s", tmpDir, err)
		}
	}()

	if err := os.WriteFile(planFile.Name(), data, 0644); err != nil {
		return err
	}

	planFile.Close()

	tf, err := tfexec.NewTerraform(tmpDir, c.terraform)
	if err != nil {
		return err
	}

	result, err := tf.ShowPlanFileRaw(context.TODO(), planFile.Name())
	if err != nil {
		return err
	}

	fmt.Fprintln(out, result)

	return nil
}
