package runner_test

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"os"
	"path/filepath"
	"testing"

	"github.com/fluxcd/pkg/tar"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/flux-iac/tofu-controller/runner"
)

const encryptionToken = "eyJhbGciOiJSUzI1NiIsImtpZCI6ImJVM0xaLXN3OUJYRFNEejF3THl2X3VvSGxoOWlHdXhYNHdTdV9Vc2w4QjAifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJ0ZXJyYWZvcm0iLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlY3JldC5uYW1lIjoidGYtcnVubmVyLWVuY3J5cHRpb24iLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC5uYW1lIjoidGYtcnVubmVyIiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZXJ2aWNlLWFjY291bnQudWlkIjoiYjJiMTc4MjMtMWI5Yi00YzEzLThhMDctNmE0OThmNjUwYjM3Iiwic3ViIjoic3lzdGVtOnNlcnZpY2VhY2NvdW50OnRlcnJhZm9ybTp0Zi1ydW5uZXIifQ.s2T3_Yd-PNF0dJO-7sP_yKbohCP-GTWrHPACUQs0nQrD3hMjZTXm-CgQdtzuKPN0fPHp_GJ8iDpWrqMRcZSqHKVSXscfCI7-QnGjqwSvt-8KBMGE7J29tFgFy6-K6uvP_kYAaA5i4bDWPXHytLmOHJj7GL_D4-0XXVB3EmCfzwREl19FdjZnmEf8lB4gJ7aOZQnW6FzJHcdzo3bUwh-S0zrjGkGsBbrBBu5hyhCKoyZP1ufn8X9NQfkdtC29rEYgI_6o2gbQrGZRdIujAVgh3HJaU2bodV4sGAdgqsMDHEeoyGzp4LSlSrR2kAYJJznF0bMFY18eojbNXnmoIpkMEQ"

func TestCreateWorkspaceBlob(t *testing.T) {
	g := NewGomegaWithT(t)

	tempDir := t.TempDir()

	encKeySecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tf-runner.cache-encryption",
			Namespace: "flux-system",
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": "tf-runner",
			},
		},
		Data: map[string][]byte{
			"token": []byte(encryptionToken),
		},
		Type: v1.SecretTypeServiceAccountToken,
	}
	err := k8sClient.Create(ctx, encKeySecret)
	g.Expect(err).To(BeNil())

	terraformDir := filepath.Join(tempDir, ".terraform")
	err = os.Mkdir(terraformDir, 0755)
	g.Expect(err).To(BeNil())

	filePath := filepath.Join(terraformDir, "random.txt")
	randomContent := []byte("random content")
	err = os.WriteFile(filePath, randomContent, 0644)
	g.Expect(err).To(BeNil())

	resp, err := runnerClient.CreateWorkspaceBlob(ctx, &runner.CreateWorkspaceBlobRequest{TfInstance: "test", WorkingDir: tempDir, Namespace: "flux-system"})
	g.Expect(err).To(BeNil())

	g.Expect(resp.GetSha256Checksum()).ToNot(BeEmpty())

	token := encKeySecret.Data["token"]
	key := token[:runner.EncryptionKeyLength]

	// Decrypting content.
	aesCipher, err := aes.NewCipher(key)
	g.Expect(err).To(BeNil())
	gcm, err := cipher.NewGCM(aesCipher)
	g.Expect(err).To(BeNil())
	nonceSize := gcm.NonceSize()
	nonce, ciphertext := resp.Blob[:nonceSize], resp.Blob[nonceSize:]
	blob, err := gcm.Open(nil, nonce, ciphertext, nil)
	g.Expect(err).To(BeNil())

	blobReader := bytes.NewReader(blob)

	outputTempDir := t.TempDir()
	err = tar.Untar(blobReader, outputTempDir)
	g.Expect(err).To(BeNil())

	outputFilePath := filepath.Join(outputTempDir, ".terraform", "random.txt")
	outputContent, err := os.ReadFile(outputFilePath)
	g.Expect(err).To(BeNil())

	g.Expect(outputContent).To(Equal(randomContent))
}
