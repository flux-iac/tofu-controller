package controllers

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/flux-iac/tofu-controller/utils"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	. "github.com/onsi/gomega"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000130_destroy_no_outputs_test(t *testing.T) {
	Spec("Terraform object with no backend, auto approve, will be reconciled to have available outputs.")

	// when("creating a Terraform object with the auto approve mode, and having a GitRepository attached to it.")
	It("should obtain the TF program's blob from the Source, unpack it, plan it, and apply it correctly with an output available, and there must be *NO* tfstate Secret because no backend specified.")
	const (
		sourceName    = "test-tf-controller-destroy-no-output"
		terraformName = "helloworld-destroy-no-outputs"
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
			URL:            server.URL() + "/tf-k8s-configmap.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:c3bf30bad9621b5110a3761a70754170d1dae6c525a63098b6ec9919efac3555", // must be the real checksum value
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
			Destroy:     false,
			Path:        "./tf-k8s-configmap",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Interval: metav1.Duration{Duration: time.Second * 10},
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
			"HasEncodingAnnotation": tfplanSecret.Annotations["encoding"] != "" && tfplanSecret.Annotations["encoding"] == "gzip",
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
					"Destroy?":        createdHelloWorldTF.Status.Plan.IsDestroyPlan,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"Message":         "Applied successfully",
		"LastAppliedPlan": "plan-master-b8e362c206",
		"Destroy?":        false,
	}))
	// TODO check Output condition

	By("checking that we have outputs available in the TF object")
	g.Eventually(func() []string {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		return createdHelloWorldTF.Status.AvailableOutputs
	}, timeout, interval).Should(Equal([]string{"api_host"}))

	// We're testing out side Kubernetes
	// so we have to disable in-cluster backend
	if os.Getenv("DISABLE_TF_K8S_BACKEND") == "1" {
		// So we're expecting that there must be no "tfstate-default-${terraformName}" secret
		By("getting the tfstate and we should see it")
		tfStateKey := types.NamespacedName{Namespace: "flux-system", Name: "tfstate-default-" + terraformName}
		tfStateSecret := corev1.Secret{}
		g.Eventually(func() string {
			err := k8sClient.Get(ctx, tfStateKey, &tfStateSecret)
			if err != nil {
				return err.Error()
			}
			return tfStateSecret.Name
		}, timeout, interval).Should(Equal("tfstate-default-helloworld-destroy-no-outputs"))
	} else {
		// TODO there's must be the default tfstate secret
	}

	cmPayloadKey := types.NamespacedName{Namespace: "default", Name: "cm-" + terraformName}
	var cmPayload corev1.ConfigMap
	g.Eventually(func() string {
		err := k8sClient.Get(ctx, cmPayloadKey, &cmPayload)
		if err != nil {
			return ""
		}
		return cmPayload.Name
	}, timeout, interval).Should(Equal("cm-" + terraformName))

	It("By updating TF object setting destroy=true to trigger the planning")
	g.Expect(k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)).Should(Succeed())
	createdHelloWorldTF.Spec.Destroy = true
	g.Expect(k8sClient.Update(ctx, &createdHelloWorldTF)).Should(Succeed())

	It("should create the destroy plan, then apply")
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
					"Destroy?":        createdHelloWorldTF.Status.Plan.IsDestroyPlan,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"Message":         "Destroy applied successfully",
		"LastAppliedPlan": "plan-master-b8e362c206",
		"Destroy?":        true,
	}))

	outputKey := types.NamespacedName{Namespace: "flux-system", Name: "tf-output-" + terraformName}
	outputSecret := corev1.Secret{}
	It("should destroy the output")
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		if apierrors.IsNotFound(err) {
			return true
		}
		return false
	}, timeout, interval).Should(BeTrue())

	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, cmPayloadKey, &cmPayload)
		if apierrors.IsNotFound(err) {
			return true
		}
		return false
	}, timeout, interval).Should(BeTrue())

	It("should delete the TF object")
	By("deleting the TF object")
	g.Expect(k8sClient.Delete(ctx, &createdHelloWorldTF)).Should(Succeed())

	It("should not have the TF object anymore")
	By("checking that the hello world TF got deleted")
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &helloWorldTF)
		if apierrors.IsNotFound(err) {
			return true
		}
		return false
	}, timeout, interval).Should(BeTrue())

}
