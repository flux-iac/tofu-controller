package controllers

import (
	"fmt"
	"testing"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"context"
	"time"
)

// +kubebuilder:docs-gen:collapse=Imports

func dependencyObject(t *testing.T) (*sourcev1.GitRepository, *infrav1.Terraform) {
	const (
		sourceName    = "gs-hcl-values-depends-on"
		terraformName = "tf-hcl-values-depends-on"
	)
	g := NewWithT(t)
	ctx := context.Background()

	By("creating a new Git repository object")
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
			Interval: metav1.Duration{Duration: time.Second * 30},
		},
	}
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())

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
			URL:            server.URL() + "/tf-hcl-values-advanced-example.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:ff0f2a1576923ec52c3a77baa59357baf7985f97fb7cccafcbb76626de409b54",
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
	var helloWorldTF infrav1.Terraform
	err := helloWorldTF.FromBytes([]byte(fmt.Sprintf(`
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: %s
  namespace: flux-system
spec:
  values:
    cluster_name: my-cluster
    access_key: my-access-key
    secret_key: ${{ .secret_key.hello_world }}
  dependsOn:
  - name: tf-depends-on
  readInputsFromSecrets:
  - name: tf-depends-on-outputs
    as: secret_key
  path: ./tf-hcl-values-advanced-example
  sourceRef:
    kind: GitRepository
    name: %s
    namespace: flux-system
  interval: 10s
  retryInterval: 10s
  approvePlan: auto
  writeOutputsToSecret:
    name: tf-output-%s
    outputs:
    - values_json_blob
`, terraformName, sourceName, terraformName)), runnerServer.Scheme)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())

	return &testRepo, &helloWorldTF
}

func Test_000350_depends_on_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource that is planned and manually approved.")
	It("should be reconciled to become planned.")
	It("should wait for a manually approval.")
	It("then should be reconciled to the applied state.")

	const (
		sourceName    = "gr-depends-on"
		terraformName = "tf-depends-on"
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
			Interval: metav1.Duration{Duration: time.Second * 30},
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

		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the status and its URL gets reconciled.")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, &createdRepo)).Should(Succeed())

	Given("a Terraform resource with manual approval, attached to the given GitRepository resource")
	By("creating a new TF resource without specifying the .spec.approvePlan field.")
	By("attaching the TF resource to the repo via `sourceRef`.")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			// Note that we do not specify the `ApprovePlan` field
			Path: "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Interval: metav1.Duration{Duration: time.Second * 10},
			WriteOutputsToSecret: &infrav1.WriteOutputsToSecretSpec{
				Name: terraformName + "-outputs",
			},
		},
	}
	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) }()

	By("checking that the TF resource existed inside the cluster.")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	By("checking that the TF's status conditions got reconciled.")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return -1
		}
		return len(createdHelloWorldTF.Status.Conditions)
	}, timeout*3, interval).ShouldNot(BeZero())

	Given("the plan id is the `plan` plus the branch name (master) plus the commit id.")
	const planId = "plan-master-b8e362c206"

	By("checking that the planned status of the TF is created successfully.")
	By("checking the reason is `TerraformPlannedWithChanges`.")
	By("checking the pending plan is the $planId.")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Plan" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Pending": createdHelloWorldTF.Status.Plan.Pending,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    infrav1.ConditionTypePlan,
		"Reason":  "TerraformPlannedWithChanges",
		"Pending": planId,
	}))

	By("checking the message of the ready status contains $planId.")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Ready" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return nil
	}, timeout*3, interval).Should(Equal(map[string]interface{}{
		"Type":    "Ready",
		"Reason":  "TerraformPlannedWithChanges",
		"Message": "Plan generated: set approvePlan: \"plan-master-b8e362c206\" to approve this plan.",
	}))

	By("checking that the planned secret is created.")
	By("checking that the label of the planned secret is the $planId.")
	tfplanKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformName}
	tfplanSecret := corev1.Secret{}
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, tfplanKey, &tfplanSecret)
		if err != nil {
			return nil
		}
		return map[string]interface{}{
			"SavedPlan":             tfplanSecret.Annotations["savedPlan"],
			"TFPlanEmpty":           string(tfplanSecret.Data["tfplan"]) == "",
			"HasEncodingAnnotation": tfplanSecret.Annotations["encoding"] == "gzip",
		}
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"SavedPlan":             planId,
		"TFPlanEmpty":           false,
		"HasEncodingAnnotation": true,
	}))

	// Before approve the first one, we add the second one as a dependency

	dependencyRepo, dependencyTF := dependencyObject(t)
	defer func() {
		g.Expect(k8sClient.Delete(ctx, dependencyTF)).Should(Succeed())
		g.Expect(k8sClient.Delete(ctx, dependencyRepo)).Should(Succeed())
	}()

	g.Eventually(func() map[string]interface{} {
		createdDependencyTF := infrav1.Terraform{}
		key := types.NamespacedName{Namespace: dependencyTF.Namespace, Name: dependencyTF.Name}
		err := k8sClient.Get(ctx, key, &createdDependencyTF)
		if err != nil {
			return nil
		}
		for _, c := range createdDependencyTF.Status.Conditions {
			if c.Type == "Ready" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return nil
	}, timeout*3, interval).Should(Equal(map[string]interface{}{
		"Type":    "Ready",
		"Reason":  infrav1.DependencyNotReadyReason,
		"Message": "dependency 'flux-system/tf-depends-on' is not ready",
	}))

	// Then we approve the first one
	By("setting the .spec.approvePlan to be plan-main- and a part of commit id (b8e362c206) to approve the plan.")
	g.Expect(k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)).Should(Succeed())
	createdHelloWorldTF.Spec.Interval = metav1.Duration{Duration: time.Hour * 10}
	createdHelloWorldTF.Spec.ApprovePlan = "plan-master-b8e362c206"
	g.Expect(k8sClient.Update(ctx, &createdHelloWorldTF)).Should(Succeed())

	It("should continue the reconciliation process to the apply state.")
	By("checking that the applied status reason is TerraformAppliedSucceed.")
	By("checking that the last applied plan is really the pending plan.")
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
					"LastAppliedPlan": createdHelloWorldTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"LastAppliedPlan": planId,
	}))
	// TODO check Output condition

	It("should contain a list of available outputs in the status.")
	By("checking that .status.availableOutput in the TF resource.")
	g.Eventually(func() []string {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		return createdHelloWorldTF.Status.AvailableOutputs
	}, timeout, interval).Should(Equal([]string{"hello_world"}))

	It("should contain the output value in the secret.")
	By("checking that the output secret is created.")
	g.Eventually(func() bool {
		tfOutputSecret := corev1.Secret{}
		tfOutputKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName + "-outputs"}
		err := k8sClient.Get(ctx, tfOutputKey, &tfOutputSecret)
		return err == nil
	}, timeout, interval).Should(BeTrue())

	g.Eventually(func() map[string]interface{} {
		createdDependencyTF := infrav1.Terraform{}
		key := types.NamespacedName{Namespace: dependencyTF.Namespace, Name: dependencyTF.Name}
		err := k8sClient.Get(ctx, key, &createdDependencyTF)
		if err != nil {
			return nil
		}
		for _, c := range createdDependencyTF.Status.Conditions {
			if c.Type == "Ready" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return nil
	}, timeout*3, interval).Should(Equal(map[string]interface{}{
		"Type":    "Ready",
		"Reason":  "TerraformOutputsWritten",
		"Message": "Outputs written: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
	}))

}
