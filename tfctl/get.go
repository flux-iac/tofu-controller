package tfctl

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/hako/durafmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Get prints information about terraform resources
func (c *CLI) Get(out io.Writer) error {
	terraformList := &infrav1.TerraformList{}
	if err := c.client.List(context.TODO(), terraformList, client.InNamespace(c.namespace)); err != nil {
		return err
	}

	if len(terraformList.Items) == 0 {
		fmt.Fprintf(out, "No resources found in %s namespace\n", c.namespace)
		return nil
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

		age := time.Now().Sub(terraform.CreationTimestamp.Time)

		data = append(data, []string{
			terraform.Namespace,
			terraform.Name,
			string(readyCondition.Status),
			shorten(readyCondition.Message),
			strconv.FormatBool(terraform.Status.Plan.Pending != ""),
			durafmt.Parse(age).LimitFirstN(1).String(),
		})
	}

	header := []string{"Namespace", "Name", "Ready", "Message", "Plan Pending", "Age"}
	table := newTablePrinter(out, header)
	table.AppendBulk(data)
	table.Render()

	return nil
}

func shorten(message string) string {
	// get the last 40 characters of the message
	var sha string
	if len(message) > 40 {
		// find / in the message
		slash := strings.LastIndex(message, "/")
		if slash != -1 {
			sha = message[slash+1:]
			// check by converting hex string to bytes
			_, err := hex.DecodeString(sha)
			if err != nil {
				return message
			}
			return message[:slash+8]
		}
	}

	return message
}
