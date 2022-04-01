package tfctl

import (
	"bytes"
	"context"
	"errors"
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
func (c *CLI) Uninstall(out io.Writer) error {
	ctx := context.Background()

	var deployment appsv1.Deployment
	if err := c.client.Get(ctx, types.NamespacedName{
		Namespace: c.namespace,
		Name:      "tf-controller",
	}, &deployment); err != nil {
		if apierrors.IsNotFound(err) {
			fmt.Fprintf(out, "tf-controller not found in %s namespace.\n", c.namespace)
			return nil
		}
		return err
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
		return errors.New("could not determine tf-controller version")
	}

	manager, err := newManager(c.client)
	if err != nil {
		return err
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

	spinner, err := yacspin.New(spinConfig)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			spinner.StopFail()
		}
		spinner.Stop()
	}()

	spinner.Start()

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

		err = manager.WaitForTermination(objects, ssa.DefaultWaitOptions())
		if err != nil {
			return err
		}
	}

	return nil
}
