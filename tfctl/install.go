package tfctl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/fluxcd/pkg/ssa"
	"github.com/theckman/yacspin"
)

// Install installs the tf-controller resources into the cluster.
func (c *CLI) Install(out io.Writer, version string, export bool) (retErr error) {
	if version == "" {
		version = c.release
	}

	manager, retErr := newManager(c.client)
	if retErr != nil {
		return retErr
	}

	if !export {
		spinConfig := yacspin.Config{
			Frequency:     100 * time.Millisecond,
			SpinnerAtEnd:  true,
			CharSet:       yacspin.CharSets[9],
			Message:       fmt.Sprintf("Installing tf-controller in %s namespace ", c.namespace),
			StopMessage:   fmt.Sprintf("tf-controller %s installed ", version),
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
	}

	for _, k := range []string{"crds", "rbac", "deployment"} {
		data, retErr := download(version, k)
		if retErr != nil {
			return retErr
		}

		if export {
			fmt.Fprintln(out, string(data))
			continue
		}

		objects, retErr := ssa.ReadObjects(bytes.NewReader(data))
		if retErr != nil {
			return retErr
		}

		_, retErr = manager.ApplyAll(context.TODO(), objects, ssa.DefaultApplyOptions())
		if retErr != nil {
			return retErr
		}

		retErr = manager.Wait(objects, ssa.DefaultWaitOptions())
		if retErr != nil {
			return retErr
		}
	}

	return nil
}
