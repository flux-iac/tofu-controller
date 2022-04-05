package tfctl

import (
	"context"
	"fmt"
	"io"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Version prints the installed version of tf-controller and the tfctl cli
func (c *CLI) Version(out io.Writer) error {
	deployment := &appsv1.Deployment{}
	if err := c.client.Get(context.TODO(), types.NamespacedName{
		Namespace: c.namespace,
		Name:      "tf-controller",
	}, deployment); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to get tf-controller deployment: %w", err)
	}

	var (
		version       string
		runnerVersion string
	)

	if len(deployment.Spec.Template.Spec.Containers) > 0 {
		for _, c := range deployment.Spec.Template.Spec.Containers {
			if c.Name == "manager" {
				version = strings.Split(c.Image, ":")[1]
				for _, e := range c.Env {
					if e.Name == "RUNNER_POD_IMAGE" {
						ref := strings.Split(e.Value, ":")
						if len(ref) == 2 {
							runnerVersion = ref[1]
						}
						break
					}
				}
				break
			}
		}
	}

	fmt.Fprintf(out, "tf-controller:\n  manager: %s\n  runner: %s\n", version, runnerVersion)
	fmt.Fprintf(out, "tfctl:\n  build: %s\n  release: %s\n", c.build, c.release)
	return nil
}
