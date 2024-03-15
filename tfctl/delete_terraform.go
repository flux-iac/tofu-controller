package tfctl

import (
	"context"
	"fmt"
	"io"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"k8s.io/apimachinery/pkg/types"
)

// DeleteTerraform deletes the terraform resource from the cluster
func (c *CLI) DeleteTerraform(out io.Writer, resource string) error {
	key := types.NamespacedName{
		Name:      resource,
		Namespace: c.namespace,
	}

	terraform := &infrav1.Terraform{}
	if err := c.client.Get(context.TODO(), key, terraform); err != nil {
		return err
	}

	if err := c.client.Delete(context.TODO(), terraform); err != nil {
		return err
	}

	fmt.Fprintf(out, " deleted Terraform resource %s/%s\n", c.namespace, resource)

	return nil
}
