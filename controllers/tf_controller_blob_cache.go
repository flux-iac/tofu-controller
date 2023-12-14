package controllers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	ctrl "sigs.k8s.io/controller-runtime"
)

// TODO: This path is hard-coded in the Deployment of the controller as mount
// point. Why not /tmp? Good question, it's a ro mount so we can't write there.
const cacheDir = "/blob-cache"

func (r *TerraformReconciler) GetWorkspaceBlobCache(ctx context.Context, runnerClient runner.RunnerClient, terraform *infrav1.Terraform, tfInstance, workdir string) error {
	log := ctrl.LoggerFrom(ctx).WithValues("step", "get workspace blob cache")

	log.Info("request workspace blob from runner", "workdir", workdir, "tfInstance", tfInstance)
	streamClient, err := runnerClient.CreateWorkspaceBlobStream(ctx, &runner.CreateWorkspaceBlobRequest{
		TfInstance: tfInstance,
		WorkingDir: workdir,
		Namespace:  terraform.Namespace,
	})
	if err != nil {
		return err
	}

	fs := NewLocalFilesystem(cacheDir)
	sha := sha256.New()
	checksum := []byte{}

	// TODO: This file pattern needs some love, it's there as a placeholder.
	// It would be beneficial if we can add the commit hash to the filename, but
	// then it would be problematic to retrieve when the source is not available,
	// and that's one of the reasons why we do this in the first place, so we can
	// get the cached content even if source is not available.
	//
	// NOTE: We can use commit hash from Source and if it's not available use from
	// lastAppliedRevision, lastAttemptedRevision, or lastPlannedRevision.
	file, err := fs.GetWriter(fmt.Sprintf("%s-%s.tar.gz", terraform.GetNamespace(), terraform.GetName()))
	if err != nil {
		return err
	}
	defer file.Close()

	for {
		chunk, err := streamClient.Recv()
		if err != nil {
			if err == io.EOF {
				if err := streamClient.CloseSend(); err != nil {
					log.Error(err, "unabel to close stream")
					break
				}
			}

			return err
		}

		if len(chunk.Blob) > 0 {
			if _, err := sha.Write(chunk.Blob); err != nil {
				return err
			}

			if _, err := file.Write(chunk.Blob); err != nil {
				return err
			}
		}

		if len(chunk.Sha256Checksum) > 0 {
			checksum = chunk.GetSha256Checksum()
		}
	}

	log.Info("calculating checksum")
	sum := sha.Sum(nil)

	if !bytes.Equal(sum, checksum) {
		return fmt.Errorf("invalid checksum, got: '%x'; expected: '%x'", sum, checksum)
	}

	return nil
}
