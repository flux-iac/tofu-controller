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
func (c *CLI) Install(out io.Writer, version string, export bool) (err error) {
	if version == "" {
		version = c.release
	}

	manager, err := newManager(c.client)
	if err != nil {
		return err
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
	}

	for i, k := range []string{"crds", "rbac", "deployment"} {
		data, err := download(version, k)
		if err != nil {
			return err
		}

		if export {
			if i > 0 {
				fmt.Fprintln(out, "---")
			}
			fmt.Fprintln(out, string(data))
			continue
		}

		objects, err := ssa.ReadObjects(bytes.NewReader(data))
		if err != nil {
			return err
		}

		_, err = manager.ApplyAll(context.TODO(), objects, ssa.DefaultApplyOptions())
		if err != nil {
			return err
		}

		err = manager.Wait(objects, ssa.DefaultWaitOptions())
		if err != nil {
			return err
		}
	}

	return nil
}
