package controllers

import (
	"context"
	"encoding/json"
	"github.com/weaveworks/tf-controller/runner"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000082_varsfrom_accepts_many_configmaps_with_last_supplied_precedence(t *testing.T) {

	const (
		terraformName     = "tf-vars-from-many-config-maps-precedence"
		generatedVarsFile = "generated.auto.tfvars.json"
	)

	g := NewWithT(t)
	ctx := context.Background()

	// By("setting up some variables")
	configMapDatas := []struct {
		name string
		data map[string]string
	}{
		{
			name: terraformName + "-config-map-1",
			data: map[string]string{
				"key-1": "value-1",
				"key-2": "value-2",
			},
		},
		{
			name: terraformName + "-config-map-2",
			data: map[string]string{
				"key-3": "value-3",
				"key-1": "value-4",
			},
		},
	}

	By("create the configmaps")
	for _, configMapData := range configMapDatas {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapData.name,
				Namespace: "flux-system",
			},
			Data: configMapData.data,
		}
		g.Expect(k8sClient.Create(ctx, configMap)).Should(Succeed())
	}

	By("creating a temporary working directory")
	workDir, err := os.MkdirTemp("", terraformName+"*")
	g.Expect(err).Should(BeNil())

	By("looking up the path of the terraform binary")
	execPath, err := exec.LookPath("terraform")
	g.Expect(err).Should(BeNil())

	By("creating a new TF exec instance")
	_, err = runnerServer.NewTerraform(ctx, &runner.NewTerraformRequest{
		WorkingDir: workDir,
		ExecPath:   execPath,
	})
	g.Expect(err).Should(BeNil())

	By("creating a new TF resource with slice of ConfigMaps")
	var varsRef []infrav1.VarsReference
	for _, configMapData := range configMapDatas {
		vr := infrav1.VarsReference{
			Kind: "ConfigMap",
			Name: configMapData.name,
		}
		if configMapData.name == terraformName+"-config-map-2" {
			vr.VarsKeys = []string{"key-1"}
		}
		varsRef = append(varsRef, vr)
	}
	terraform := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			VarsFrom: varsRef,
			WriteOutputsToSecret: &infrav1.WriteOutputsToSecretSpec{
				Name: "tf-output-" + terraformName,
				Outputs: []string{
					"hello_world",
				},
			},
		},
	}

	terraformBytes, err := terraform.ToBytes(reconciler.Scheme)
	g.Expect(err).To(BeNil())

	_, err = runnerServer.Init(ctx, &runner.InitRequest{
		TfInstance: "1",
		Upgrade:    false,
		ForceCopy:  false,
		Terraform:  terraformBytes,
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
	}
	g.Expect(vars).Should(Equal(expectedResult))
}
