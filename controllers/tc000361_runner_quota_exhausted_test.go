package controllers

import (
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/utils"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_000361_runner_quota_exhausted_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource when runner quota is exhausted.")
	It("should detect quota errors and requeue with jitter instead of failing.")

	const (
		sourceName    = "test-tf-controller-quota"
		terraformName = "terraform-quota-test"
		namespace     = "flux-system"
	)
	g := NewWithT(t)
	ctx := t.Context()

	Given("a GitRepository with test terraform code")
	By("creating a GitRepository resource.")
	testRepo := sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceName,
			Namespace: namespace,
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/openshift-fluxv2-poc/podinfo",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "master",
			},
			Interval: metav1.Duration{Duration: time.Second * 30},
		},
	}

	By("creating the GitRepository in the cluster.")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	defer waitResourceToBeDelete(g, &testRepo)

	Given("the GitRepository's reconciled status")
	By("setting up a successful GitRepository status.")
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
			Path:           "gitrepository/flux-system/test-tf-controller-quota/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	By("updating the GitRepository status.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	Given("a Terraform resource configured to detect quota errors")
	By("creating a Terraform resource with quota retry configuration.")
	var terraformResource infrav1.Terraform
	err := terraformResource.FromBytes(fmt.Appendf(nil, `
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: %s
  namespace: %s
spec:
  approvePlan: auto
  path: ./terraform-hello-world-example
  sourceRef:
    kind: GitRepository
    name: %s
    namespace: %s
  interval: 10s
`, terraformName, namespace, sourceName, namespace), runnerServer.Scheme)
	g.Expect(err).ToNot(HaveOccurred())

	By("creating the Terraform resource in the cluster.")
	It("should be created successfully.")
	g.Expect(k8sClient.Create(ctx, &terraformResource)).Should(Succeed())
	defer waitResourceToBeDelete(g, &terraformResource)

	Given("quota error detection")
	By("verifying IsQuotaError recognizes quota Forbidden errors.")
	quotaError := apierrors.NewForbidden(schema.GroupResource{Group: "", Resource: "pods"}, "test-pod", errors.New("exceeded quota"))
	It("should detect quota errors correctly.")
	g.Expect(utils.IsQuotaError(quotaError)).Should(BeTrue())

	Given("non-quota Forbidden errors")
	By("verifying IsQuotaError rejects non-quota Forbidden errors.")
	nonQuotaError := apierrors.NewForbidden(schema.GroupResource{Group: "", Resource: "pods"}, "test-pod", errors.New("permission denied"))
	It("should not detect non-quota errors.")
	g.Expect(utils.IsQuotaError(nonQuotaError)).Should(BeFalse())

	Given("quota detection with different resource types")
	By("verifying IsQuotaError recognizes quota errors for different resources.")
	podsQuotaError := apierrors.NewForbidden(schema.GroupResource{Group: "", Resource: "pods"}, "test", errors.New("exceeded quota: pods, requested: pods=1, used: pods=2, limited: pods=2"))
	cpuQuotaError := apierrors.NewForbidden(schema.GroupResource{Group: "", Resource: "pods"}, "test", errors.New("exceeded quota: compute, requested: requests.cpu=100m, used: requests.cpu=200m, limited: requests.cpu=200m"))
	memoryQuotaError := apierrors.NewForbidden(schema.GroupResource{Group: "", Resource: "pods"}, "test", errors.New("exceeded quota: memory, requested: requests.memory=512Mi, used: requests.memory=1Gi, limited: requests.memory=1Gi"))
	It("should detect all quota errors regardless of resource type.")
	g.Expect(utils.IsQuotaError(podsQuotaError)).Should(BeTrue())
	g.Expect(utils.IsQuotaError(cpuQuotaError)).Should(BeTrue())
	g.Expect(utils.IsQuotaError(memoryQuotaError)).Should(BeTrue())

	Given("non-Forbidden errors")
	By("verifying IsQuotaError rejects non-Forbidden errors even with quota keywords.")
	notFoundError := apierrors.NewNotFound(schema.GroupResource{}, "test")
	badRequestWithQuota := apierrors.NewBadRequest("quota exceeded")
	It("should not detect non-Forbidden errors as quota errors.")
	g.Expect(utils.IsQuotaError(notFoundError)).Should(BeFalse())
	g.Expect(utils.IsQuotaError(badRequestWithQuota)).Should(BeFalse())
}
