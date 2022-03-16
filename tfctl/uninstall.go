package tfctl

import (
	"bytes"
	"context"
	"strings"

	"github.com/fluxcd/pkg/ssa"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Uninstall removes the tf-controller resources from the cluster.
func (c *CLI) Uninstall() error {
	ctx := context.Background()

	var deployment appsv1.Deployment
	if err := c.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      "tf-controller",
	}, &deployment); err != nil {
		return err
	}

	version := strings.Split(deployment.Spec.Template.Spec.Containers[0].Image, ":")[1]

	manager, err := newManager(c.client)
	if err != nil {
		return err
	}

	for _, k := range []string{"crds", "rbac", "deployment"} {
		data, err := download(version, k)
		if err != nil {
			return err
		}

		objects, err := ssa.ReadObjects(bytes.NewReader(data))
		if err != nil {
			return err
		}

		_, err = manager.DeleteAll(context.TODO(), objects, ssa.DefaultDeleteOptions())
		if err != nil {
			return err
		}
	}

	return nil
}
