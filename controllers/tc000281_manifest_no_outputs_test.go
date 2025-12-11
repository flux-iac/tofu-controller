package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/flux-iac/tofu-controller/utils"

	. "github.com/onsi/gomega"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000281_manifest_no_outputs_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource when applying manifest with no defined outputs.")
	It("should be reconciled successfully with a correct status conditions on initial apply.")

	const (
		sourceName    = "gr-manifest-no-outputs"
		terraformName = "tf-manifest-no-outputs"
	)
	g := NewWithT(t)
	ctx := context.Background()

	Given("a GitRepository")
	By("defining a new GitRepository object")
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
	By("creating the GitRepository object")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	defer waitResourceToBeDelete(g, &testRepo)

	Given("that the GitRepository got reconciled")
	By("setting the GitRepository's status, with the BLOB's URL, and the correct checksum")
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

		Artifact: &meta.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/tf-k8s-configmap-no-output.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:28237d2e6590220901e51993e8efc785f57a4bf3e6b0f10e35ac015737e154b9", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	testEnvKubeConfigPath, err := findKubeConfig(testEnv)
	g.Expect(err).Should(BeNil())

	Given("a Terraform object with auto approve, and attaching it to the GitRepository object")
	By("creating a new TF resource and attaching to the repo via sourceRef")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			BackendConfig: &infrav1.BackendConfigSpec{
				SecretSuffix:    terraformName,
				InClusterConfig: false,
				ConfigPath:      testEnvKubeConfigPath,
			},
			ApprovePlan: "auto",
			Interval:    metav1.Duration{Duration: 1 * time.Hour}, // Long interval to test only initial apply
			Path:        "./tf-k8s-configmap-no-output",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Vars: []infrav1.Variable{
				{
					Name:  "kubeconfig",
					Value: utils.MustJSONEncodeBytes(t, []byte(testEnvKubeConfigPath)),
				},
				{
					Name:  "context",
					Value: utils.MustJSONEncodeBytes(t, []byte("envtest")),
				},
				{
					Name:  "config_name",
					Value: utils.MustJSONEncodeBytes(t, []byte("cm-"+terraformName)),
				},
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer waitResourceToBeDelete(g, &helloWorldTF)
	defer waitResourceToBeDelete(g, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "cm-" + terraformName},
	})

	It("should be created")
	By("checking that the hello world TF got created")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	It("should have conditions reconciled")
	By("checking that the hello world TF's status conditions has some elements")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return -1
		}
		return len(createdHelloWorldTF.Status.Conditions)
	}, timeout, interval).ShouldNot(BeZero())

	It("should have its plan reconciled")
	By("checking that the Plan's Status of the TF program is Planned Succeed.")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Plan" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return createdHelloWorldTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    infrav1.ConditionTypePlan,
		"Reason":  "TerraformPlannedWithChanges",
		"Message": "Plan generated",
	}))

	It("should generate the Secret containing the plan named with branch and commit id")
	By("checking that the Secret contains plan-master-b8e362c206 in its labels")
	tfplanKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformName}
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
		"SavedPlan":             "plan-master-b8e362c206",
		"Is TFPlan empty ?":     false,
		"HasEncodingAnnotation": true,
	}))

	By("checking that the applied status of the TF program Successfully, and plan-master-b8e3 is applied")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"Message":         c.Message,
					"LastAppliedPlan": createdHelloWorldTF.Status.Plan.LastApplied,
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

	It("should have the expected conditions.")
	By("checking that the TF resource's status conditions match the expected conditions.")
	g.Eventually(func() []normalisedCondition {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		return normaliseConditions(createdHelloWorldTF.Status.Conditions)
	}, timeout, interval).Should(Equal([]normalisedCondition{
		{Type: meta.ReadyCondition, Status: metav1.ConditionTrue, Reason: infrav1.TFExecApplySucceedReason, Message: "Applied successfully: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb"},
		{Type: infrav1.ConditionTypeApply, Status: metav1.ConditionTrue, Reason: infrav1.TFExecApplySucceedReason, Message: "Applied successfully"},
		{Type: infrav1.ConditionTypePlan, Status: metav1.ConditionTrue, Reason: infrav1.PlannedWithChangesReason, Message: "Plan generated"},
	}))
}
