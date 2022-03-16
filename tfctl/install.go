package tfctl

import (
	"bytes"
	"context"

	"github.com/fluxcd/pkg/ssa"
)

// Install installs the tf-controller resources into the cluster.
func (c *CLI) Install(version string) error {
	if version == "" {
		version = "v0.9.0-rc.8" //TODO(piaras): retrieve this from build tag or api call
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

		objects, err := ssa.ReadObjects(bytes.NewReader(data))
		if err != nil {
			return err
		}

		_, err = manager.ApplyAll(context.TODO(), objects, ssa.DefaultApplyOptions())
		if err != nil {
			return err
		}
	}

	return nil
}
