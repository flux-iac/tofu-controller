//go:build skip
// +build skip

package controllers

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000220_support_config_file_via_secret_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource that has a config file attached.")
	It("should generate the .tfrc file, and point the environment to that file so that the terraform binary could pick the correct configuration.")

	var (
		sourceName    = "tfrc-gitrepo-no-output"
		terraformName = "tfrc-helloworld-no-outputs" + rand.String(6)
	)

	const (
		// override timeout just for this test
		timeout = time.Second * 150
	)

	g := NewWithT(t)
	ctx := context.Background()

	Given("a GitRepository")
	By("defining a new GitRepository resource.")
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
			Interval: metav1.Duration{Duration: time.Second * 30},
		},
	}
	By("creating the GitRepository resource in the cluster.")
	It("should be created successfully.")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &testRepo)).Should(Succeed()) }()

	Given("the GitRepository's reconciled status.")
	By("setting the GitRepository's status, with the downloadable BLOB's URL, and the correct checksum.")
	updatedTime := time.Now()
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

		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/tfc-helloworld.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:e236bcf665fd3e186cd0d8908d589c37735111b03cd2a67e9c07695e318e9ae5", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("preparing a Secret containing a tfrc fie")
	tfrcSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "terraform-cli-config",
			Namespace: "flux-system",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"terraform.tfrc": []byte(fmt.Sprintf(`
credentials "app.terraform.io" {
  token = "%s"
}
`, os.Getenv("TOKEN_FROM_TF"))),
		},
	}
	g.Expect(k8sClient.Create(ctx, &tfrcSecret)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &tfrcSecret)).Should(Succeed()) }()

	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Interval:    metav1.Duration{Duration: 10 * time.Minute},
			BackendConfig: &infrav1.BackendConfigSpec{
				Disable: true,
			},
			CliConfigSecretRef: &corev1.SecretReference{
				Name:      "terraform-cli-config",
				Namespace: "flux-system",
			},
			Path: "./tfc-helloworld",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
		},
	}

	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) }()

	By("checking that the TF resource existed inside the cluster.")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &helloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	It("should be planned.")
	By("checking that the Plan's reason of the TF resource become `TerraformPlannedWithChanges`.")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &helloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range helloWorldTF.Status.Conditions {
			if c.Type == "Plan" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return helloWorldTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    infrav1.ConditionTypePlan,
		"Reason":  "TerraformPlannedWithChanges",
		"Message": "Plan generated",
	}))

	It("should generate the Secret containing the plan named with branch and commit id.")
	By("checking that the Secret contains plan-master-b8e362c206e3d0cbb7ed22ced771a0056455a2fb in its labels.")
	tfplanKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformName}
	tfplanSecret := corev1.Secret{}
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, tfplanKey, &tfplanSecret)
		if err != nil {
			return nil
		}
		return map[string]interface{}{
			"SavedPlan":         tfplanSecret.Annotations["savedPlan"],
			"Is TFPlan empty ?": string(tfplanSecret.Data["tfplan"]) == "",
		}
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"SavedPlan":         "plan-master-b8e362c206",
		"Is TFPlan empty ?": false,
	}))

	It("should contain an Apply condition saying that the plan were apply successfully.")
	By("checking that the reason of the Apply condition is TerraformAppliedSucceed, and the LastAppliedPlan is the plan.")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &helloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range helloWorldTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"Message":         c.Message,
					"LastAppliedPlan": helloWorldTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"Message":         "Applied successfully",
		"LastAppliedPlan": "plan-master-b8e362c206",
	}))

	g.Expect(k8sClient.Get(ctx, helloWorldTFKey, &helloWorldTF)).Should(Succeed())
	helloWorldTF.Spec.Destroy = true
	g.Expect(k8sClient.Update(ctx, &helloWorldTF)).Should(Succeed())

	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &helloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range helloWorldTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"Message":         c.Message,
					"LastAppliedPlan": helloWorldTF.Status.Plan.LastApplied,
					"IsDestroy":       helloWorldTF.Status.Plan.IsDestroyPlan,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"Message":         "Applied successfully",
		"LastAppliedPlan": "plan-master-b8e362c206",
		"IsDestroy":       true,
	}))

}
