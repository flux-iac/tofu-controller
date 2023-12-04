package runner

import (
	context "context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/weaveworks/tf-controller/internal/storage"
	ctrl "sigs.k8s.io/controller-runtime"
)

// CreateWorkspaceBlob archives and compresses using tar and gzip the .terraform directory and returns the tarball as a byte array
func (r *TerraformRunnerServer) CreateWorkspaceBlob(ctx context.Context, req *CreateWorkspaceBlobRequest) (*CreateWorkspaceBlobReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	archivePath, err := storage.ArchiveDir(filepath.Join(req.WorkingDir, ".terraform"))
	if err != nil {
		log.Error(err, "unable to archive .terraform directory")
		return nil, err
	}

	// read archivePath into byte array
	blob, err := os.ReadFile(archivePath)
	if err != nil {
		log.Error(err, "unable to read archive file")
		return nil, err
	}

	sha := sha256.New()
	if _, err := sha.Write(blob); err != nil {
		return &CreateWorkspaceBlobReply{Blob: blob}, err
	}
	sum := sha.Sum(nil)

	return &CreateWorkspaceBlobReply{
		Blob:           blob,
		Sha256Checksum: sum,
	}, nil
}
