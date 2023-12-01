package runner_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/fluxcd/pkg/untar"
	. "github.com/onsi/gomega"
	"github.com/weaveworks/tf-controller/runner"
)

func TestCreateWorkspaceBlob(t *testing.T) {
	g := NewGomegaWithT(t)

	tempDir := t.TempDir()

	terraformDir := filepath.Join(tempDir, ".terraform")
	err := os.Mkdir(terraformDir, 0755)
	g.Expect(err).To(BeNil())

	filePath := filepath.Join(terraformDir, "random.txt")
	randomContent := []byte("random content")
	err = os.WriteFile(filePath, randomContent, 0644)
	g.Expect(err).To(BeNil())

	resp, err := runnerClient.CreateWorkspaceBlob(ctx, &runner.CreateWorkspaceBlobRequest{TfInstance: "test", WorkingDir: tempDir})
	g.Expect(err).To(BeNil())

	blobReader := bytes.NewReader(resp.Blob)

	outputTempDir := t.TempDir()
	untar.Untar(blobReader, outputTempDir)

	outputFilePath := filepath.Join(outputTempDir, ".terraform", "random.txt")
	outputContent, err := os.ReadFile(outputFilePath)
	g.Expect(err).To(BeNil())

	g.Expect(outputContent).To(Equal(randomContent))
}
