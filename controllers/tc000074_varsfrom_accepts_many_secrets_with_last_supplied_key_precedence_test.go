package controllers

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/flux-iac/tofu-controller/runner"
	"github.com/google/uuid"

	. "github.com/onsi/gomega"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000074_varsfrom_accepts_many_secrets_with_last_supplied_key_precedence(t *testing.T) {
	const (
		terraformName     = "tf-vars-from-many-secrets-precedence"
		generatedVarsFile = "generated.auto.tfvars.json"
	)

	g := NewWithT(t)
	ctx := context.Background()

	// By("setting up some variables")
	secretDatas := []struct {
		name string
		data map[string]string
	}{
		{
			name: terraformName + "-secret-1",
			data: map[string]string{
				"key-1": "value-1",
				"key-2": "value-2",
			},
		},
		{
			name: terraformName + "-secret-2",
			data: map[string]string{
				"key-3": "value-3",
				"key-1": "value-4",
			},
		},
	}

	By("create the secrets")
	for _, secret := range secretDatas {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secret.name,
				Namespace: "flux-system",
			},
			StringData: secret.data,
		}
		g.Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
		defer waitResourceToBeDelete(g, secret)
	}

	By("creating a temporary working directory")
	workDir, err := os.MkdirTemp("", terraformName+"*")
	g.Expect(err).Should(BeNil())

	By("looking up the path of the terraform binary")
	execPath, err := exec.LookPath("terraform")
	g.Expect(err).Should(BeNil())

	By("creating a new TF resource with slice of ConfigMaps")
	var varsRef []infrav1.VarsReference
	for _, secretData := range secretDatas {
		varsRef = append(varsRef, infrav1.VarsReference{
			Kind: "Secret",
			Name: secretData.name,
		})
	}

	terraform := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			VarsFrom: varsRef,
		},
	}

	terraformBytes, err := terraform.ToBytes(reconciler.Scheme)
	g.Expect(err).To(BeNil())

	By("creating a new TF exec instance")
	ntResp, err := runnerServer.NewTerraform(ctx, &runner.NewTerraformRequest{
		WorkingDir: workDir,
		ExecPath:   execPath,
		InstanceID: uuid.New().String(),
		Terraform:  terraformBytes,
	})
	g.Expect(err).Should(BeNil())

	_, err = runnerServer.Init(ctx, &runner.InitRequest{
		TfInstance: ntResp.Id,
		Upgrade:    false,
		ForceCopy:  false,
	})
	g.Expect(err).Should(BeNil())

	_, err = runnerServer.GenerateVarsForTF(ctx, &runner.GenerateVarsForTFRequest{
		WorkingDir: workDir,
	})
	g.Expect(err).Should(BeNil())

	By("verifying the generated vars file matches the expected result")
	varsFilePath := filepath.Join(workDir, generatedVarsFile)

	// read vars file
	data, err := os.ReadFile(varsFilePath)
	g.Expect(err).Should(BeNil())

	// unmarshal data
	var vars map[string]string
	g.Expect(json.Unmarshal(data, &vars)).Should(Succeed())

	// check vars
	expectedResult := map[string]string{
		"key-1": "value-4",
		"key-2": "value-2",
		"key-3": "value-3",
	}
	g.Expect(vars).Should(Equal(expectedResult))
}
