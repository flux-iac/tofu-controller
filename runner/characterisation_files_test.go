package runner_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/flux-iac/tofu-controller/runner"

	. "github.com/onsi/gomega"
)

func TestCharacterisation_ProcessCliConfig(t *testing.T) {
	process := func(h *harness, dir string) (*runner.ProcessCliConfigReply, error) {
		return h.Client.ProcessCliConfig(t.Context(), &runner.ProcessCliConfigRequest{
			DirPath:   dir,
			Namespace: "default",
			Name:      "cli",
		})
	}

	t.Run("one tfrc key", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t, secret("cli", map[string]string{"app.tfrc": "credentials {}"}))
		dir := t.TempDir()

		reply, err := process(h, dir)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(reply.FilePath).To(Equal(filepath.Join(dir, "app.tfrc")))
		got, err := os.ReadFile(reply.FilePath)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(string(got)).To(Equal("credentials {}"))
	})

	t.Run("two keys", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t, secret("cli", map[string]string{"a.tfrc": "1", "b.tfrc": "2"}))

		_, err := process(h, t.TempDir())

		g.Expect(status.Code(err)).To(Equal(codes.Internal))
		g.Expect(status.Convert(err).Message()).To(Equal("expect the secret to contain 1 data"))
	})

	t.Run("key without the tfrc extension", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t, secret("cli", map[string]string{"config": "1"}))
		dir := t.TempDir()

		_, err := process(h, dir)

		g.Expect(status.Code(err)).To(Equal(codes.Internal))
		g.Expect(status.Convert(err).Message()).To(Equal("expect the secret key to end with .tfrc"))
		g.Expect(filepath.Join(dir, "config")).NotTo(BeAnExistingFile())
	})
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCharacterisation_UploadAndExtract(t *testing.T) {
	t.Run("path inside the archive", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)

		reply, err := h.Client.UploadAndExtract(t.Context(), &runner.UploadAndExtractRequest{
			Namespace: "default",
			Name:      "app",
			TarGz:     tarGz(t, map[string]string{"sub/main.tf": "# main"}),
			Path:      "sub",
		})

		g.Expect(err).NotTo(HaveOccurred())
		tmpDir := filepath.Join(h.Server.TempDir, "default-app")
		g.Expect(reply.TmpDir).To(Equal(tmpDir))
		g.Expect(reply.WorkingDir).To(Equal(filepath.Join(tmpDir, "sub")))
		g.Expect(filepath.Join(reply.WorkingDir, "main.tf")).To(BeAnExistingFile())
	})

	t.Run("path missing from the archive", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)

		_, err := h.Client.UploadAndExtract(t.Context(), &runner.UploadAndExtractRequest{
			Namespace: "default",
			Name:      "app",
			TarGz:     tarGz(t, map[string]string{"main.tf": "# main"}),
			Path:      "nope",
		})

		g.Expect(status.Code(err)).To(Equal(codes.Internal))
		missing := filepath.Join(h.Server.TempDir, "default-app", "nope")
		g.Expect(status.Convert(err).Message()).To(Equal(
			"terraform path not found: stat " + missing + ": no such file or directory"))
	})
}

func TestCharacterisation_CreateFileMappings(t *testing.T) {
	t.Run("parent traversal stays under the root", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)
		workspace := filepath.Join(h.Dir, "workspace")
		g.Expect(os.MkdirAll(workspace, 0o755)).To(Succeed())

		_, err := h.Client.CreateFileMappings(t.Context(), &runner.CreateFileMappingsRequest{
			WorkingDir: workspace,
			FileMappings: []*runner.FileMapping{
				{Location: "workspace", Path: "../escape", Content: []byte("x")},
			},
		})

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(filepath.Join(workspace, "escape")).To(BeAnExistingFile())
		g.Expect(filepath.Join(h.Dir, "escape")).NotTo(BeAnExistingFile())
	})

	t.Run("unknown location", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)

		_, err := h.Client.CreateFileMappings(t.Context(), &runner.CreateFileMappingsRequest{
			FileMappings: []*runner.FileMapping{
				{Location: "elsewhere", Path: "f", Content: []byte("x")},
			},
		})

		g.Expect(status.Code(err)).To(Equal(codes.Internal))
		g.Expect(status.Convert(err).Message()).To(Equal("unknown file mapping location"))
	})
}

func TestCharacterisation_GenerateTemplate(t *testing.T) {
	t.Run("renders main.tf.tpl", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)
		wd := t.TempDir()
		id := h.newSession(t, newTerraform("app"), newStubTerraform(t).Path(), wd)
		g.Expect(os.WriteFile(filepath.Join(wd, "main.tf.tpl"),
			[]byte(`name = "{{ .Values.x | upper }}"`), 0o644)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(wd, "generated.auto.tfvars.json"),
			[]byte(`{"values":{"x":"hello"}}`), 0o644)).To(Succeed())

		reply, err := h.Client.GenerateTemplate(t.Context(), &runner.GenerateTemplateRequest{TfInstance: id, WorkingDir: wd})

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(reply.Message).To(Equal("ok"))
		got, err := os.ReadFile(filepath.Join(wd, "main.tf"))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(string(got)).To(Equal(`name = "HELLO"`))
	})

	t.Run("no template", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)
		wd := t.TempDir()
		id := h.newSession(t, newTerraform("app"), newStubTerraform(t).Path(), wd)

		reply, err := h.Client.GenerateTemplate(t.Context(), &runner.GenerateTemplateRequest{TfInstance: id, WorkingDir: wd})

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(reply.Message).To(Equal("ok"))
		g.Expect(filepath.Join(wd, "main.tf")).NotTo(BeAnExistingFile())
	})
}
