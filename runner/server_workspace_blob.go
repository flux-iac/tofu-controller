package runner

import (
	context "context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"github.com/flux-iac/tofu-controller/internal/storage"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const EncryptionKeyLength = 32

// CreateWorkspaceBlobStream archives and compresses using tar and gzip the
// workspace directory and returns the tarball as a byte array.
func (r *TerraformRunnerServer) CreateWorkspaceBlobStream(req *CreateWorkspaceBlobRequest, streamServer Runner_CreateWorkspaceBlobStreamServer) error {
	log := ctrl.Log
	// We dont' have context here... that's not good.
	// log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return err
	}

	sum, err := r.archiveAndEncrypt(
		context.Background(),
		req.Namespace,
		req.WorkingDir,
		func(chunk []byte) error {
			return streamServer.Send(&CreateWorkspaceBlobReply{Blob: chunk})
		},
	)
	if err != nil {
		log.Error(err, "unable to archive and encrypt wokspace cache")
		return err
	}

	return streamServer.Send(&CreateWorkspaceBlobReply{
		Blob:           []byte{},
		Sha256Checksum: sum,
	})
}

func (r *TerraformRunnerServer) archiveAndEncrypt(ctx context.Context, namespace, path string, chunkFn func([]byte) error) ([]byte, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)

	log.Info("archiving workspace directory", "dir", path)
	archivePath, err := storage.ArchiveDir(path)
	if err != nil {
		return nil, fmt.Errorf("unable to archive workspace directory: %w", err)
	}

	// Read encryption secret.
	secretName := "tf-runner.cache-encryption"
	encryptionSecretKey := types.NamespacedName{Name: secretName, Namespace: namespace}
	var encryptionSecret v1.Secret

	log.Info("fetching secret key", "key", encryptionSecretKey)
	if err := r.Client.Get(ctx, encryptionSecretKey, &encryptionSecret); err != nil {
		return nil, fmt.Errorf("unable to get encryption secret: %w", err)
	}

	// 256 bit AES encryption with Galois Counter Mode.
	log.Info("encrypting content")
	token := encryptionSecret.Data["token"]
	key := token[:EncryptionKeyLength]

	// Read archivePath into byte array.
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("unable to read archive file: %w", err)
	}

	// AES
	aesCipher, _ := aes.NewCipher(key)
	sha := sha256.New()

	// Generate and send IV.
	iv := make([]byte, aes.BlockSize)
	_, err = rand.Read(iv)
	if err != nil {
		return nil, fmt.Errorf("failed to read random data as iv: %w", err)
	}

	chunkFn(iv)

	// Read, encrypt, and send.
	for {
		block := make([]byte, aes.BlockSize)
		n, err := file.Read(block)
		if err != nil && err != io.EOF {
			return nil, err
		}

		if n == 0 {
			break
		}

		ciphertext := make([]byte, aes.BlockSize)

		stream := cipher.NewCTR(aesCipher, iv)
		stream.XORKeyStream(ciphertext, block)

		if _, err := sha.Write(ciphertext); err != nil {
			return nil, fmt.Errorf("unable to write sha256 checksum: %w", err)
		}

		chunkFn(ciphertext)
	}

	return sha.Sum(nil), nil
}
