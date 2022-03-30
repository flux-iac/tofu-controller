package tfctl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fluxcd/pkg/ssa"
	"github.com/theckman/yacspin"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// Uninstall removes the tf-controller resources from the cluster.
func (c *CLI) Uninstall(out io.Writer) (retErr error) {
	ctx := context.Background()

	var deployment appsv1.Deployment
	if retErr := c.client.Get(ctx, types.NamespacedName{
		Namespace: c.namespace,
		Name:      "tf-controller",
	}, &deployment); retErr != nil {
		if apierrors.IsNotFound(retErr) {
			fmt.Fprintf(out, "tf-controller not found.\n")
			return nil
		}
		return retErr
	}

	var version string
	if len(deployment.Spec.Template.Spec.Containers) > 0 {
		for _, c := range deployment.Spec.Template.Spec.Containers {
			if c.Name == "manager" {
				version = strings.Split(c.Image, ":")[1]
				break
			}
		}
	}

	if version == "" {
		return fmt.Errorf("could not determine tf-controller version")
	}

	manager, retErr := newManager(c.client)
	if retErr != nil {
		return retErr
	}

	spinConfig := yacspin.Config{
		Frequency:     100 * time.Millisecond,
		CharSet:       yacspin.CharSets[9],
		SpinnerAtEnd:  true,
		Message:       "Uninstalling tf-controller ",
		StopMessage:   fmt.Sprintf("tf-controller %s uninstalled ", version),
		StopCharacter: "✓",
		Colors:        []string{"yellow"},
		StopColors:    []string{"fgGreen"},
	}

	spinner, retErr := yacspin.New(spinConfig)
	if retErr != nil {
		return retErr
	}

	defer func() {
		if retErr != nil {
			spinner.StopFail()
		}
		spinner.Stop()
	}()

	spinner.Start()

	for _, k := range []string{"crds", "rbac", "deployment"} {
		data, retErr := download(version, k)
		if retErr != nil {
			return retErr
		}

		objects, retErr := ssa.ReadObjects(bytes.NewReader(data))
		if retErr != nil {
			return retErr
		}

		_, retErr = manager.DeleteAll(context.TODO(), objects, ssa.DefaultDeleteOptions())
		if retErr != nil {
			return retErr
		}

		retErr = manager.WaitForTermination(objects, ssa.DefaultWaitOptions())
		if retErr != nil {
			return retErr
		}
	}

	return nil
}
