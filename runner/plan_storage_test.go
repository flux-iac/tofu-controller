package runner

import (
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/flux-iac/tofu-controller/api/plan"
	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	. "github.com/onsi/gomega"
	"github.com/spf13/afero"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLoadTFPlanWithForceTrue(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()
	fs := afero.NewMemMapFs()

	req := &LoadTFPlanRequest{
		Name:                     "test",
		Namespace:                "default",
		PendingPlan:              "plan-is-1",
		BackendCompletelyDisable: false,
	}

	terraform := &infrav1.Terraform{
		Spec: infrav1.TerraformSpec{
			Force: true,
		},
	}

	const workingDir = "/tmp"

	data, err := plan.GzipEncode([]byte("plan data")) // Assuming GzipEncode returns a byte slice
	g.Expect(err).NotTo(HaveOccurred(), "should not return an error")

	secretData := map[string][]byte{
		TFPlanName: data,
	}

	tfplanSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tfplan-" + terraform.WorkspaceName() + "-" + req.Name,
			Namespace: req.Namespace,
			Annotations: map[string]string{
				SavedPlanSecretAnnotation:          "plan-not-1",
				plan.TFPlanFullWorkspaceAnnotation: terraform.WorkspaceName(),
			},
			Labels: map[string]string{
				plan.TFPlanNameLabel:      plan.SafeLabelValue(req.Name),
				plan.TFPlanWorkspaceLabel: plan.SafeLabelValue(terraform.WorkspaceName()),
			},
		},
		Data: secretData,
	}

	// Mock the Kubernetes client.
	client := fake.NewClientBuilder().WithObjects(tfplanSecret).Build()

	// Act: Call the function under test.
	reply, loadPlanErr := loadTFPlan(ctx, req, terraform, workingDir, client, fs)

	// Assert: Check that the function behaved as expected.
	g.Expect(loadPlanErr).NotTo(HaveOccurred(), "should not return an error")
	g.Expect(reply).To(Equal(&LoadTFPlanReply{Message: "ok"}), "should return the expected reply")

	// Assert: Check that the expected file was written to the mock filesystem.
	expectedData, _ := plan.GzipDecode(secretData[TFPlanName])
	actualData, _ := afero.ReadFile(fs, filepath.Join(workingDir, TFPlanName))
	g.Expect(actualData).To(Equal(expectedData), "should write the expected data to the filesystem")
}

func TestLoadTFPlanWithForceFalse(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()
	fs := afero.NewMemMapFs()

	req := &LoadTFPlanRequest{
		Name:                     "test",
		Namespace:                "default",
		PendingPlan:              "plan-is-1",
		BackendCompletelyDisable: false,
	}

	terraform := &infrav1.Terraform{
		Spec: infrav1.TerraformSpec{
			Force: false,
		},
	}

	const workingDir = "/tmp"

	data, err := plan.GzipEncode([]byte("plan data")) // Assuming GzipEncode returns a byte slice
	g.Expect(err).NotTo(HaveOccurred(), "should not return an error")

	secretData := map[string][]byte{
		TFPlanName: data,
	}

	tfplanSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tfplan-" + terraform.WorkspaceName() + "-" + req.Name,
			Namespace: req.Namespace,
			Annotations: map[string]string{
				SavedPlanSecretAnnotation:          "plan-not-1",
				plan.TFPlanFullWorkspaceAnnotation: terraform.WorkspaceName(),
			},
			Labels: map[string]string{
				plan.TFPlanNameLabel:      plan.SafeLabelValue(req.Name),
				plan.TFPlanWorkspaceLabel: plan.SafeLabelValue(terraform.WorkspaceName()),
			},
		},
		Data: secretData,
	}

	// Mock the Kubernetes client.
	client := fake.NewClientBuilder().WithObjects(tfplanSecret).Build()

	// Act: Call the function under test.
	reply, loadPlanErr := loadTFPlan(ctx, req, terraform, workingDir, client, fs)

	// Assert: loadPlanErr should contain an error
	g.Expect(loadPlanErr).To(HaveOccurred(), "should return an error")

	// Assert: reply should be nil
	g.Expect(reply).To(BeNil(), "should return nil reply")
}

func TestWritePlanAsSecretReportsSavedObjects(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	server := &TerraformRunnerServer{Client: fake.NewClientBuilder().Build()}

	tfPlan, err := plan.NewFromBytes("a-plan", "terraform", "default", "plan-uuid-1", "plan-abc", []byte("plan data"))
	g.Expect(err).NotTo(HaveOccurred())

	saved, err := server.writePlanAsSecret(ctx, "a-plan", "terraform", "default", tfPlan, "")
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(saved).To(HaveLen(1))
	g.Expect(saved[0].ApiVersion).To(Equal("v1"))
	g.Expect(saved[0].Kind).To(Equal("Secret"))
	g.Expect(saved[0].Namespace).To(Equal("terraform"))
	g.Expect(saved[0].Name).To(Equal("tfplan-default-a-plan"))

	// api/plan is the single place that owns the ownerReference; the runner
	// must not add a second one pointing at the same Terraform.
	written := &corev1.Secret{}
	g.Expect(server.Get(ctx, types.NamespacedName{Namespace: "terraform", Name: saved[0].Name}, written)).To(Succeed())
	g.Expect(written.OwnerReferences).To(HaveLen(1))
	g.Expect(written.OwnerReferences[0].Kind).To(Equal(infrav1.TerraformKind))
	g.Expect(string(written.OwnerReferences[0].UID)).To(Equal("plan-uuid-1"))
}

func TestWritePlanAsSecretReportsEveryChunk(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	server := &TerraformRunnerServer{Client: fake.NewClientBuilder().Build()}

	// Genuinely random, so it does not compress below the chunk threshold. A
	// patterned filler would gzip to a single chunk and silently skip the case
	// this test exists for.
	planData := make([]byte, 4*1024*1024)
	_, err := rand.Read(planData)
	g.Expect(err).NotTo(HaveOccurred())

	tfPlan, err := plan.NewFromBytes("a-large-plan", "terraform", "default", "plan-uuid-1", "plan-abc", planData)
	g.Expect(err).NotTo(HaveOccurred())

	expected, err := tfPlan.ToSecret("")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(len(expected)).To(BeNumerically(">", 1), "test needs a chunked plan")

	saved, err := server.writePlanAsSecret(ctx, "a-large-plan", "terraform", "default", tfPlan, "")
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(saved).To(HaveLen(len(expected)), "every chunk must be reported so status.chunks is accurate")
	g.Expect(saved[0].Name).To(Equal(expected[0].Name), "the first entry must be chunk 0")
}
