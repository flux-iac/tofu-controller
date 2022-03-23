package tfctl

import (
	"context"
	"io"
	"strconv"

	"github.com/fluxcd/pkg/apis/meta"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// GetTerraform prints information about the provided resource
func (c *CLI) GetTerraform(out io.Writer, resource string) error {
	key := types.NamespacedName{
		Name:      resource,
		Namespace: c.namespace,
	}

	terraform := &infrav1.Terraform{}
	if err := c.client.Get(context.TODO(), key, terraform); err != nil {
		return err
	}

	var readyCondition metav1.Condition
	for _, cond := range *terraform.GetStatusConditions() {
		if cond.Type == meta.ReadyCondition {
			readyCondition = cond
			break
		}
	}

	header := []string{"Name", "Ready", "Message", "Drift Detected", "Suspended"}

	table := newTablePrinter(out, header)

	table.Append([]string{
		terraform.Name,
		string(readyCondition.Status),
		readyCondition.Message,
		strconv.FormatBool(terraform.HasDrift()),
		strconv.FormatBool(terraform.Spec.Suspend),
	})

	table.Render()

	return nil
}
