package controllers

import (
	"context"
	"fmt"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/utils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

func Test_000300_targets(t *testing.T) {
	const (
		sourceName    = "test-tf-controller-targets"
		terraformName = "tf-multi-resources-targets"

		resourceOne = "kubernetes_config_map_v1.one"
		resourceTwo = "kubernetes_config_map_v1.two"

		configMapOne = "cm-" + terraformName + "-one"
		configMapTwo = "cm-" + terraformName + "-two"
	)

	Spec("This spec describes the behavior of Terraform with targets.")

	g := NewWithT(t)
	ctx := context.Background()
	updatedTime := time.Now()

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
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &testRepo)).Should(Succeed()) }()

	By("setting the git repo status object, the URL, and the correct checksum")
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
			URL:            server.URL() + "/tf-multi-resources.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:d304b84675a595928753e123dcdc2b04f7f4cc57f91510593fb64bd00b242d25",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the status and its URL gets reconciled")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := &sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, createdRepo)).Should(Succeed())

	testEnvKubeConfigPath, err := findKubeConfig(testEnv)
	g.Expect(err).Should(BeNil())

	By("creating a new TF with targets set to one resource and attaching to the repo")
	applyTargetTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./controllers/data/tf-multi-resources",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Interval:                   metav1.Duration{Duration: 5 * time.Second},
			EnableInventory:            true,
			DestroyResourcesOnDeletion: true,
			Targets:                    []string{resourceOne},
			BackendConfig: &infrav1.BackendConfigSpec{
				SecretSuffix:    terraformName,
				InClusterConfig: false,
				ConfigPath:      testEnvKubeConfigPath,
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
	g.Expect(k8sClient.Create(ctx, &applyTargetTF)).Should(Succeed())

	applyTargetTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdApplyTargetTF := infrav1.Terraform{}

	It("should have conditions reconciled")
	By("checking that the TF's status conditions has some elements")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, applyTargetTFKey, &createdApplyTargetTF)
		if err != nil {
			return -1
		}
		return len(createdApplyTargetTF.Status.Conditions)
	}, timeout, interval).ShouldNot(BeZero())

	It("should have its plan reconciled")
	By("checking that the Plan's Status of the TF program is Planned Succeed.")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, applyTargetTFKey, &createdApplyTargetTF)
		if err != nil {
			return nil
		}
		for _, c := range createdApplyTargetTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return createdApplyTargetTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    "Apply",
		"Reason":  infrav1.TFExecApplySucceedReason,
		"Message": "Applied successfully",
	}))

	By("checking that the multi resources TF got created and contains one resource in the inventory")
	g.Eventually(func() []string {
		err := k8sClient.Get(ctx, applyTargetTFKey, &createdApplyTargetTF)
		if err != nil {
			return nil
		}
		var results []string
		for _, entry := range createdApplyTargetTF.Status.Inventory.Entries {
			results = append(results, fmt.Sprintf("%s.%s", entry.Type, entry.Name))
		}
		return results
	}, timeout, interval).Should(Equal([]string{resourceOne}))

	It("should create one config map")
	cmPayloadKey := types.NamespacedName{Namespace: "default", Name: configMapOne}
	var cmPayload corev1.ConfigMap
	g.Eventually(func() string {
		err := k8sClient.Get(ctx, cmPayloadKey, &cmPayload)
		if err != nil {
			return ""
		}
		return cmPayload.Name
	}, timeout, interval).Should(Equal(configMapOne))

	By("removing the targets on the apply")
	It("should create all resources")
	g.Eventually(func() error {
		err = k8sClient.Get(ctx, applyTargetTFKey, &createdApplyTargetTF)
		if err != nil {
			return nil
		}
		createdApplyTargetTF.Spec.Targets = nil
		return k8sClient.Update(ctx, &createdApplyTargetTF)
	}, timeout, interval).Should(BeNil())

	It("should have its plan reconciled")
	By("checking that the Plan's Status of the TF program is Planned Succeed.")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, applyTargetTFKey, &createdApplyTargetTF)
		if err != nil {
			return nil
		}
		for _, c := range createdApplyTargetTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return createdApplyTargetTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    "Apply",
		"Reason":  infrav1.TFExecApplySucceedReason,
		"Message": "Applied successfully",
	}))

	By("checking that all resources are listed in the inventory")
	g.Eventually(func() []string {
		err := k8sClient.Get(ctx, applyTargetTFKey, &createdApplyTargetTF)
		if err != nil {
			return nil
		}
		var results []string
		for _, entry := range createdApplyTargetTF.Status.Inventory.Entries {
			results = append(results, fmt.Sprintf("%s.%s", entry.Type, entry.Name))
		}
		return results
	}, timeout, interval).Should(Equal([]string{resourceOne, resourceTwo}))

	By("checking that config map resource one exists")
	cmPayloadKey = types.NamespacedName{Namespace: "default", Name: configMapOne}
	cmPayload = corev1.ConfigMap{}
	g.Eventually(func() string {
		err := k8sClient.Get(ctx, cmPayloadKey, &cmPayload)
		if err != nil {
			return ""
		}
		return cmPayload.Name
	}, timeout, interval).Should(Equal(configMapOne))

	By("Checking that config map resource two exists")
	cmPayloadKey = types.NamespacedName{Namespace: "default", Name: configMapTwo}
	cmPayload = corev1.ConfigMap{}
	g.Eventually(func() string {
		err := k8sClient.Get(ctx, cmPayloadKey, &cmPayload)
		if err != nil {
			return ""
		}
		return cmPayload.Name
	}, timeout, interval).Should(Equal(configMapTwo))

	Given("a Terraform object with targets set to resource two")
	patch := client.MergeFrom(applyTargetTF.DeepCopy())
	applyTargetTF.Spec.Targets = []string{resourceTwo}
	g.Expect(k8sClient.Patch(ctx, &applyTargetTF, patch)).Should(Succeed())

	By("deleting TF object to trigger the destroy planning")
	createdApplyTargetTF = infrav1.Terraform{}
	g.Expect(k8sClient.Get(ctx, applyTargetTFKey, &createdApplyTargetTF)).Should(Succeed())
	g.Expect(k8sClient.Delete(ctx, &createdApplyTargetTF)).Should(Succeed())

	It("should create the destroy plan, then apply")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, applyTargetTFKey, &createdApplyTargetTF)
		if err != nil {
			return nil
		}
		for _, c := range createdApplyTargetTF.Status.Conditions {
			if c.Type == "Plan" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return createdApplyTargetTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    "Plan",
		"Reason":  "TerraformPlannedWithChanges",
		"Message": "Plan generated",
	}))

	cmPayloadKey = types.NamespacedName{Namespace: "default", Name: configMapTwo}
	cmPayload = corev1.ConfigMap{}
	It("should destroy resource two")
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, cmPayloadKey, &cmPayload)
		if apierrors.IsNotFound(err) {
			return true
		}
		return false
	}, timeout, interval).Should(BeTrue())

	It("should NOT destroy resource one")
	cmPayloadKey = types.NamespacedName{Namespace: "default", Name: configMapOne}
	cmPayload = corev1.ConfigMap{}
	g.Eventually(func() string {
		err := k8sClient.Get(ctx, cmPayloadKey, &cmPayload)
		if err != nil {
			return ""
		}
		return cmPayload.Name
	}, timeout, interval).Should(Equal(configMapOne))

}
