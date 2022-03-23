package tfctl

import (
	"context"
	"io"
	"strconv"

	"github.com/fluxcd/pkg/apis/meta"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Get prints information about terraform resources
func (c *CLI) Get(out io.Writer) error {
	terraformList := &infrav1.TerraformList{}
	if err := c.client.List(context.TODO(), terraformList, client.InNamespace(c.namespace)); err != nil {
		return err
	}

	var data [][]string
	for _, terraform := range terraformList.Items {
		var readyCondition metav1.Condition
		for _, cond := range *terraform.GetStatusConditions() {
			if cond.Type == meta.ReadyCondition {
				readyCondition = cond
				break
			}
		}
		data = append(data, []string{
			terraform.Name,
			string(readyCondition.Status),
			readyCondition.Message,
			strconv.FormatBool(terraform.HasDrift()),
			strconv.FormatBool(terraform.Spec.Suspend),
		})
	}

	header := []string{"Name", "Ready", "Message", "Drift Detected", "Suspended"}
	table := newTablePrinter(out, header)
	table.AppendBulk(data)
	table.Render()

	return nil
}
