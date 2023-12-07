package runner

import (
	context "context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/weaveworks/tf-controller/internal/storage"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const EncryptionKeyLength = 32

// CreateWorkspaceBlob archives and compresses using tar and gzip the .terraform directory and returns the tarball as a byte array
func (r *TerraformRunnerServer) CreateWorkspaceBlob(ctx context.Context, req *CreateWorkspaceBlobRequest) (*CreateWorkspaceBlobReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	log.Info("archiving workspace directory", "dir", req.WorkingDir)
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

	secretName := "tf-runner.cache-encryption"
	encryptionSecretKey := types.NamespacedName{Name: secretName, Namespace: req.Namespace}
	var encryptionSecret v1.Secret

	log.Info("fetching secret key", "key", encryptionSecretKey)
	if err := r.Client.Get(ctx, encryptionSecretKey, &encryptionSecret); err != nil {
		return nil, err
	}

	// 256 bit AES encryption with Galois Counter Mode.
	log.Info("encrypting content")
	token := encryptionSecret.Data["token"]
	key := token[:EncryptionKeyLength]

	aesCipher, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(aesCipher)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	_, err = rand.Read(nonce)
	if err != nil {
		return nil, err
	}

	out := gcm.Seal(nonce, nonce, blob, nil)

	// SHA256 checksum so we can verify if the saved content is not corrupted.
	log.Info("generating sha256 checksum")
	sha := sha256.New()
	if _, err := sha.Write(out); err != nil {
		return nil, err
	}
	sum := sha.Sum(nil)

	return &CreateWorkspaceBlobReply{
		Blob:           out,
		Sha256Checksum: sum,
	}, nil
}
