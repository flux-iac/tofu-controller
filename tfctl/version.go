package tfctl

import (
	"context"
	"fmt"
	"io"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Version prints the installed version of tf-controller and the tfctl cli
func (c *CLI) Version(out io.Writer) error {
	var deployment appsv1.Deployment
	if err := c.client.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      "tf-controller",
	}, &deployment); err != nil {
		return err
	}

	version := strings.Split(deployment.Spec.Template.Spec.Containers[0].Image, ":")[1]

	fmt.Fprintf(out, "tf-controller: %s\n", version)
	fmt.Fprintf(out, "tfctl: %s\n", c.build)

	return nil
}
