package tfctl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/fluxcd/pkg/ssa"
)

// Install installs the tf-controller resources into the cluster.
func (c *CLI) Install(out io.Writer, version string, export bool) error {
	if version == "" {
		version = c.release
	}

	manager, err := newManager(c.client)
	if err != nil {
		return err
	}

	for _, k := range []string{"crds", "rbac", "deployment"} {
		data, err := download(version, k)
		if err != nil {
			return err
		}

		if export {
			fmt.Fprintf(os.Stdout, string(data))
		} else {
			objects, err := ssa.ReadObjects(bytes.NewReader(data))
			if err != nil {
				return err
			}

			_, err = manager.ApplyAll(context.TODO(), objects, ssa.DefaultApplyOptions())
			if err != nil {
				return err
			}
		}
	}

	fmt.Fprintf(out, " Terraform controller %s installed\n", version)

	return nil
}
