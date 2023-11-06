package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000243_remediation_retry_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource that obtains a bad tar.gz file blob from its Source reference.")
	It("should report the error and stop reconcile.")

	const (
		sourceName    = "test-tf-controller-retry"
		terraformName = "retry"
	)
	g := NewWithT(t)
	ctx := context.Background()

	Given("a GitRepository")
	By("defining a new GitRepository resource")
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

	Given("the GitRepository's reconciled status.")
	By("setting the GitRepository's status, with the downloadable *bad* BLOB's URL, with the correct checksum.")
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
			URL:            server.URL() + "/bad.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:196d115c43583ccd10107d631d8a594be542a75911f9832a5ec2c1e22b65387b",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())
	defer waitResourceToBeDelete(g, &testRepo)

	Given("a Terraform object with auto approve, and attaching it to the bad GitRepository resource.")
	By("creating a new TF resource and attaching to the bad repo via `sourceRef`.")
	var helloWorldTF infrav1.Terraform
	err := helloWorldTF.FromBytes([]byte(fmt.Sprintf(`
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: %s
  namespace: flux-system
spec:
  approvePlan: auto
  path: ./terraform-hello-world-example
  remediation:
    retries: 3
  retryInterval: 5s
  sourceRef:
    kind: GitRepository
    name: %s
    namespace: flux-system
  interval: 10s
`, terraformName, sourceName)), runnerServer.Scheme)
	g.Expect(err).ToNot(HaveOccurred())

	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer waitResourceToBeDelete(g, &helloWorldTF)

	By("checking that the TF resource existed inside the cluster.")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		return err == nil
	}, timeout, interval).Should(BeTrue())

	It("should be reconciled and contain some status conditions.")
	By("checking that the TF resource's status conditions has 1 element.")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return -1
		}
		return len(createdHelloWorldTF.Status.Conditions)
	}, timeout, interval).Should(Equal(1))

	It("should be an error")
	By("checking that the Ready's reason of the TF resource become `ArtifactFailed`.")
	type failedStatusCheckResult struct {
		Type    string
		Reason  string
		Message string
		Status  metav1.ConditionStatus
		Retries int64
	}
	expected := failedStatusCheckResult{
		Type:    "Ready",
		Reason:  "ArtifactFailed",
		Message: "rpc error: code = Unknown desc = failed to untar artifact, error: requires gzip-compressed body: gzip: invalid header",
		Status:  metav1.ConditionFalse,
		Retries: 3,
	}
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Ready" {
				return failedStatusCheckResult{
					Type:    c.Type,
					Reason:  c.Reason,
					Message: c.Message,
					Status:  c.Status,
					Retries: createdHelloWorldTF.Status.ReconciliationFailures,
				}
			}
		}
		return createdHelloWorldTF.Status
	}, timeout, interval).Should(Equal(expected))

	// It should never reach 4 when Retry is set to 3.
	expected = failedStatusCheckResult{
		Retries: 4,
	}
	time.Sleep(15 * time.Second)
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Ready" {
				return failedStatusCheckResult{
					Retries: createdHelloWorldTF.Status.ReconciliationFailures,
				}
			}
		}
		return createdHelloWorldTF.Status
	}, timeout, interval).Should(Not(Equal(expected)))

	// After changing the resource, retry count should be set back to 0.
	// With setting Retries lower than the previous one, we can check if it was
	// reset to 0 as it would never reach 2 from 3.
	createdHelloWorldTF.Spec.Remediation.Retries = 2
	g.Expect(k8sClient.Update(ctx, &createdHelloWorldTF)).Should(Succeed())

	expected = failedStatusCheckResult{
		Type:    "Ready",
		Reason:  "ArtifactFailed",
		Message: "rpc error: code = Unknown desc = failed to untar artifact, error: requires gzip-compressed body: gzip: invalid header",
		Status:  metav1.ConditionFalse,
		Retries: 2,
	}
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Ready" {
				return failedStatusCheckResult{
					Type:    c.Type,
					Reason:  c.Reason,
					Message: c.Message,
					Status:  c.Status,
					Retries: createdHelloWorldTF.Status.ReconciliationFailures,
				}
			}
		}
		return createdHelloWorldTF.Status
	}, timeout, interval).Should(Equal(expected))
}
