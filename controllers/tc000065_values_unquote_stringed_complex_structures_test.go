package controllers

import (
	"context"
	"fmt"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"
	. "github.com/onsi/gomega"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_values_unquote_stringed_complex_structures_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resources which outputs a list and an object and another resource which injects these in the values field using templating")

	const (
		sourceName          = "test-tf-controller-complex-structure"
		terraformNameOutput = "tf-complex-structure-output"
		terraformNameInput  = "tf-complex-structure-input"
	)
	ctx := context.Background()
	g := NewWithT(t)

	Given("a GitRepository")
	By("defining a new GitRepository resource.")
	updatedTime := time.Now()
	testRepo := sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceName,
			Namespace: "flux-system",
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/openshift-fluxv2-poc/podinfo",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "master",
			},
			Interval:          metav1.Duration{Duration: time.Second * 30},
			GitImplementation: "go-git",
		},
	}

	By("creating the GitRepository resource in the cluster.")
	It("should be created successfully.")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &testRepo)).Should(Succeed()) }()

	Given("the GitRepository's reconciled status")
	By("setting the GitRepository's status, with the downloadable BLOB's URL, and the correct checksum.")
	testRepo.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(1),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			},
		},
		URL: server.URL() + "/tf-complex-structure.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/tf-complex-structure.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "9c714be59c0687a23c8fef228ac3fd4656b054364e2c875d8d74f63726edc714", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the status and its URL gets reconciled")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := &sourcev1.GitRepository{}
	g.Eventually(func() bool {
		_ = k8sClient.Get(ctx, gitRepoKey, createdRepo)
		for _, c := range createdRepo.Status.Conditions {
			if c.Type == "Ready" && c.Status == metav1.ConditionTrue {
				return true
			}
		}
		return false
	}, timeout, interval).Should(BeTrue())

	By("creating a new TF and attaching to the repo")

	testEnvKubeConfigPath, err := findKubeConfig(testEnv)
	g.Expect(err).Should(BeNil())

	complexStructureOutputTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformNameOutput,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			Interval:      metav1.Duration{Duration: 10 * time.Second},
			RetryInterval: &metav1.Duration{Duration: 10 * time.Second},
			ApprovePlan:   "auto",
			Path:          "./tf-complex-structure/output",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			WriteOutputsToSecret: &infrav1.WriteOutputsToSecretSpec{
				Name: "tf-outputs-" + terraformNameOutput,
			},
			BackendConfig: &infrav1.BackendConfigSpec{
				SecretSuffix:    terraformNameOutput,
				InClusterConfig: false,
				ConfigPath:      testEnvKubeConfigPath,
			},
		},
		Status: infrav1.TerraformStatus{},
	}
	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &complexStructureOutputTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &complexStructureOutputTF)).Should(Succeed()) }()

	By("checking that the TF resource existed inside the cluster.")
	complexStructureOutputTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformNameOutput}
	createdComplexStructureOutputTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, complexStructureOutputTFKey, &createdComplexStructureOutputTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	By("checking that the TF's status conditions got reconciled.")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, complexStructureOutputTFKey, &createdComplexStructureOutputTF)
		if err != nil {
			return -1
		}
		return len(createdComplexStructureOutputTF.Status.Conditions)
	}, timeout*3, interval).ShouldNot(BeZero())

	It("should be planned.")
	By("checking that the Plan's reason of the TF resource become `TerraformPlannedWithChanges`.")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, complexStructureOutputTFKey, &createdComplexStructureOutputTF)
		if err != nil {
			return nil
		}
		for _, c := range createdComplexStructureOutputTF.Status.Conditions {
			if c.Type == "Plan" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return createdComplexStructureOutputTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    infrav1.ConditionTypePlan,
		"Reason":  infrav1.PlannedWithChangesReason,
		"Message": "Plan generated",
	}))

	It("should generate the Secret containing the plan named with branch and commit id.")
	By("checking that the Secret contains plan-master-b8e362c206e3d0cbb7ed22ced771a0056455a2fb in its labels.")
	tfplanKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformNameOutput}
	tfplanSecret := corev1.Secret{}
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, tfplanKey, &tfplanSecret)
		if err != nil {
			return nil
		}
		return map[string]interface{}{
			"SavedPlan":             tfplanSecret.Annotations["savedPlan"],
			"Is TFPlan empty ?":     string(tfplanSecret.Data["tfplan"]) == "",
			"HasEncodingAnnotation": tfplanSecret.Annotations["encoding"] == "gzip",
		}
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"SavedPlan":             "plan-master-b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
		"Is TFPlan empty ?":     false,
		"HasEncodingAnnotation": true,
	}))

	It("should contain an Apply condition saying that the plan were apply successfully.")
	By("checking that the reason of the Apply condition is TerraformAppliedSucceed, and the LastAppliedPlan is the plan.")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, complexStructureOutputTFKey, &createdComplexStructureOutputTF)
		if err != nil {
			return nil
		}
		for _, c := range createdComplexStructureOutputTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"Message":         c.Message,
					"LastAppliedPlan": createdComplexStructureOutputTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"Message":         "Applied successfully",
		"LastAppliedPlan": "plan-master-b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
	}))

	It("should have an available output.")
	By("checking that the Terraform resource's .status.availableOutputs contains my_list as an output name.")
	g.Eventually(func() []string {
		err := k8sClient.Get(ctx, complexStructureOutputTFKey, &createdComplexStructureOutputTF)
		if err != nil {
			return nil
		}
		return createdComplexStructureOutputTF.Status.AvailableOutputs
	}, timeout, interval).Should(Equal([]string{"my_list", "my_object"}))

	It("should have a output")
	By("checking if a secret exist and it contains the my_list value")
	g.Eventually(func() string {
		var output corev1.Secret
		err := k8sClient.Get(ctx, types.NamespacedName{
			Namespace: "flux-system",
			Name:      createdComplexStructureOutputTF.Spec.WriteOutputsToSecret.Name,
		}, &output)
		if err != nil {
			return "nil"
		}
		return string(output.Data["my_list"])
	}, timeout, interval).Should(Equal(`["entry-1","entry-2"]`))

	var complexStructureInputTF infrav1.Terraform

	err = complexStructureInputTF.FromBytes([]byte(fmt.Sprintf(`
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: %s
  namespace: flux-system
spec:
  interval: 10s
  retryInterval: 10s
  approvePlan: auto
  path: ./tf-complex-structure/input
  sourceRef:
    kind: GitRepository
    name: %s
    namespace: flux-system
  destroyResourcesOnDeletion: true
  writeOutputsToSecret:
    name: tf-outputs-%s
  readInputsFromSecrets:
    - name: %s
      as: inputs
  values:
    my_list: ${{ .inputs.my_list }}
    my_object: ${{ .inputs.my_object }}
`, terraformNameInput, sourceName, terraformNameInput, createdComplexStructureOutputTF.Spec.WriteOutputsToSecret.Name)), runnerServer.Scheme)
	g.Expect(err).Should(Succeed())

	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &complexStructureInputTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &complexStructureInputTF)).Should(Succeed()) }()

	By("checking that the TF resource existed inside the cluster.")
	complexStructureInputTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformNameInput}
	createdComplexStructureInputTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, complexStructureInputTFKey, &createdComplexStructureInputTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	It("should contain an Apply condition saying that the plan were apply successfully.")
	By("checking that the reason of the Apply condition is TerraformAppliedSucceed, and the LastAppliedPlan is the plan.")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, complexStructureInputTFKey, &createdComplexStructureInputTF)
		if err != nil {
			return nil
		}
		for _, c := range createdComplexStructureInputTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"Message":         c.Message,
					"LastAppliedPlan": createdComplexStructureInputTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"Message":         "Applied successfully",
		"LastAppliedPlan": "plan-master-b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
	}))

	It("should have a output")
	By("checking if a secret exist and it contains the length of my_list")
	g.Eventually(func() string {
		var output corev1.Secret
		err := k8sClient.Get(ctx, types.NamespacedName{
			Namespace: "flux-system",
			Name:      createdComplexStructureInputTF.Spec.WriteOutputsToSecret.Name,
		}, &output)
		if err != nil {
			return "nil"
		}
		return string(output.Data["my_list_length"])
	}, timeout, interval).Should(Equal("2"))
}
