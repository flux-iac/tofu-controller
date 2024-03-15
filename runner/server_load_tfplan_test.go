package runner

import (
	"context"
	"path/filepath"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/utils"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"github.com/spf13/afero"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLoadTFPlanWithForceTrue(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := context.TODO()
	log := logr.Discard()
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

	data, err := utils.GzipEncode([]byte("plan data")) // Assuming GzipEncode returns a byte slice
	g.Expect(err).NotTo(HaveOccurred(), "should not return an error")

	secretData := map[string][]byte{
		TFPlanName: data,
	}

	tfplanSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "tfplan-" + terraform.WorkspaceName() + "-" + req.Name,
			Namespace:   req.Namespace,
			Annotations: map[string]string{SavedPlanSecretAnnotation: "plan-not-1"},
		},
		Data: secretData,
	}

	// Mock the Kubernetes client.
	client := fake.NewClientBuilder().WithObjects(tfplanSecret).Build()

	// Act: Call the function under test.
	reply, loadPlanErr := loadTFPlan(ctx, log, req, terraform, workingDir, client, fs)

	// Assert: Check that the function behaved as expected.
	g.Expect(loadPlanErr).NotTo(HaveOccurred(), "should not return an error")
	g.Expect(reply).To(Equal(&LoadTFPlanReply{Message: "ok"}), "should return the expected reply")

	// Assert: Check that the expected file was written to the mock filesystem.
	expectedData, _ := utils.GzipDecode(secretData[TFPlanName])
	actualData, _ := afero.ReadFile(fs, filepath.Join(workingDir, TFPlanName))
	g.Expect(actualData).To(Equal(expectedData), "should write the expected data to the filesystem")
}

func TestLoadTFPlanWithForceFalse(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := context.TODO()
	log := logr.Discard()
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

	data, err := utils.GzipEncode([]byte("plan data")) // Assuming GzipEncode returns a byte slice
	g.Expect(err).NotTo(HaveOccurred(), "should not return an error")

	secretData := map[string][]byte{
		TFPlanName: data,
	}

	tfplanSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "tfplan-" + terraform.WorkspaceName() + "-" + req.Name,
			Namespace:   req.Namespace,
			Annotations: map[string]string{SavedPlanSecretAnnotation: "plan-not-1"},
		},
		Data: secretData,
	}

	// Mock the Kubernetes client.
	client := fake.NewClientBuilder().WithObjects(tfplanSecret).Build()

	// Act: Call the function under test.
	reply, loadPlanErr := loadTFPlan(ctx, log, req, terraform, workingDir, client, fs)

	// Assert: loadPlanErr should contain an error
	g.Expect(loadPlanErr).To(HaveOccurred(), "should return an error")

	// Assert: reply should be nil
	g.Expect(reply).To(BeNil(), "should return nil reply")
}
