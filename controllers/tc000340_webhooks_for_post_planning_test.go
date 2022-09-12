package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/runner"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type mockRunnerClientForTestWebhooksForPostPlanning struct {
	runner.RunnerClient
}

func (m *mockRunnerClientForTestWebhooksForPostPlanning) ShowPlanFile(ctx context.Context, req *runner.ShowPlanFileRequest, opts ...grpc.CallOption) (*runner.ShowPlanFileReply, error) {
	return &runner.ShowPlanFileReply{
		JsonOutput: []byte(`{"dummy": "plan"}`),
	}, nil
}

func Test_000340_PrepareWebhookPayloadSpecAndPlan(t *testing.T) {
	g := NewWithT(t)
	terraform := infrav1.Terraform{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "infra.contrib.fluxcd.io/v1alpha1",
		},
	}
	err := terraform.FromBytes([]byte(fmt.Sprintf(`
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: %s
  namespace: flux-system
spec:
  approvePlan: auto
  path: ./terraform-hello-world-example
  sourceRef:
    kind: GitRepository
    name: %s
    namespace: flux-system
  interval: 10s
  webhooks:
  - stage: post-planning
    url: %s
    errorMessageTemplate: "Violation: {{ (index .violations 0).message }}"
`, "helloworld", "gitrepo", "http://localhost:8080/terraform/admission")), runnerServer.Scheme)
	g.Expect(err).ToNot(HaveOccurred())
	mockRunnerClient := &mockRunnerClientForTestWebhooksForPostPlanning{}
	payload, err := reconciler.prepareWebhookPayload(terraform, mockRunnerClient, "SpecAndPlan")
	g.Expect(err).ToNot(HaveOccurred())

	expected, err := yaml.Parse(`
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
  creationTimestamp: null
spec:
  approvePlan: auto
  path: ./terraform-hello-world-example
  runnerPodTemplate:
    metadata: {}
    spec: {}
  sourceRef:
    kind: GitRepository
    name: gitrepo
    namespace: flux-system
  interval: 10s
  webhooks:
  - stage: post-planning
    url: http://localhost:8080/terraform/admission
    errorMessageTemplate: "Violation: {{ (index .violations 0).message }}"
status:
  tfplan:
    dummy: plan
`)

	g.Expect(err).ToNot(HaveOccurred())
	expectedBytes, err := expected.MarshalJSON()
	g.Expect(err).ToNot(HaveOccurred())

	var payloadMap map[string]interface{}
	var expectedMap map[string]interface{}

	err = json.Unmarshal(payload, &payloadMap)
	g.Expect(err).ToNot(HaveOccurred())

	err = json.Unmarshal(expectedBytes, &expectedMap)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(payloadMap).To(Equal(expectedMap))
}

func Test_000340_PrepareWebhookPayloadSpecOnly(t *testing.T) {
	g := NewWithT(t)
	terraform := infrav1.Terraform{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "infra.contrib.fluxcd.io/v1alpha1",
		},
	}
	err := terraform.FromBytes([]byte(fmt.Sprintf(`
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: %s
  namespace: flux-system
spec:
  approvePlan: auto
  path: ./terraform-hello-world-example
  sourceRef:
    kind: GitRepository
    name: %s
    namespace: flux-system
  interval: 10s
  webhooks:
  - stage: post-planning
    url: %s
    errorMessageTemplate: "Violation: {{ (index .violations 0).message }}"
    payloadType: SpecOnly
`, "helloworld", "gitrepo", "http://localhost:8080/terraform/admission")), runnerServer.Scheme)
	g.Expect(err).ToNot(HaveOccurred())
	mockRunnerClient := &mockRunnerClientForTestWebhooksForPostPlanning{}
	payload, err := reconciler.prepareWebhookPayload(terraform, mockRunnerClient, "SpecOnly")
	g.Expect(err).ToNot(HaveOccurred())

	expected, err := yaml.Parse(`
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
  creationTimestamp: null
spec:
  approvePlan: auto
  path: ./terraform-hello-world-example
  runnerPodTemplate:
    metadata: {}
    spec: {}
  sourceRef:
    kind: GitRepository
    name: gitrepo
    namespace: flux-system
  interval: 10s
  webhooks:
  - stage: post-planning
    url: http://localhost:8080/terraform/admission
    errorMessageTemplate: "Violation: {{ (index .violations 0).message }}"
    payloadType: SpecOnly
`)

	g.Expect(err).ToNot(HaveOccurred())
	expectedBytes, err := expected.MarshalJSON()
	g.Expect(err).ToNot(HaveOccurred())

	var payloadMap map[string]interface{}
	var expectedMap map[string]interface{}

	err = json.Unmarshal(payload, &payloadMap)
	g.Expect(err).ToNot(HaveOccurred())

	err = json.Unmarshal(expectedBytes, &expectedMap)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(payloadMap).To(Equal(expectedMap))
}

func Test_000340_PrepareWebhookPayloadPlanOnly(t *testing.T) {
	g := NewWithT(t)
	terraform := infrav1.Terraform{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "infra.contrib.fluxcd.io/v1alpha1",
		},
	}
	err := terraform.FromBytes([]byte(fmt.Sprintf(`
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: %s
  namespace: flux-system
spec:
  approvePlan: auto
  path: ./terraform-hello-world-example
  sourceRef:
    kind: GitRepository
    name: %s
    namespace: flux-system
  interval: 10s
  webhooks:
  - stage: post-planning
    url: %s
    errorMessageTemplate: "Violation: {{ (index .violations 0).message }}"
    payloadType: PlanOnly
`, "helloworld", "gitrepo", "http://localhost:8080/terraform/admission")), runnerServer.Scheme)
	g.Expect(err).ToNot(HaveOccurred())
	mockRunnerClient := &mockRunnerClientForTestWebhooksForPostPlanning{}
	payload, err := reconciler.prepareWebhookPayload(terraform, mockRunnerClient, "PlanOnly")
	g.Expect(err).ToNot(HaveOccurred())

	expected, err := yaml.Parse(`
dummy: plan
`)

	g.Expect(err).ToNot(HaveOccurred())
	expectedBytes, err := expected.MarshalJSON()
	g.Expect(err).ToNot(HaveOccurred())

	var payloadMap map[string]interface{}
	var expectedMap map[string]interface{}

	err = json.Unmarshal(payload, &payloadMap)
	g.Expect(err).ToNot(HaveOccurred())

	err = json.Unmarshal(expectedBytes, &expectedMap)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(payloadMap).To(Equal(expectedMap))
}

func Test_000340_webhooks_for_post_planning_test(t *testing.T) {
	Spec("This spec describes the behaviour of webhooks for the post-planning stage of Terraform CR.")
	// It("should connect to the webhook server, and send a POST request to the webhook URL.")
	// It("should send the webhook request with the correct payload.")
	// It("should continue to next webhook, if the previous webhook returns a 200 status code.")

	const (
		sourceName    = "gr-w-webhooks"
		terraformName = "tf-w-webhooks"
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
			Interval:          metav1.Duration{Duration: time.Second * 30},
			GitImplementation: "go-git",
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
		URL: server.URL() + "/file.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	Given("a Terraform resource with auto approve, attached to the given GitRepository resource.")
	By("creating a new TF resource and attaching to the repo via `sourceRef`.")
	var helloWorldTF infrav1.Terraform
	err := helloWorldTF.FromBytes([]byte(fmt.Sprintf(`
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: %s
  namespace: flux-system
spec:
  approvePlan: auto
  path: ./terraform-hello-world-example
  sourceRef:
    kind: GitRepository
    name: %s
    namespace: flux-system
  interval: 10s
  webhooks:
  - stage: post-planning
    url: %s
    testExpression: "{{ .passed }}"
    errorMessageTemplate: "SHOULD PASS Violation: {{ (index .violations 0).message }}"
  - stage: post-planning
    url: %s
    testExpression: "{{ .passed }}"
    errorMessageTemplate: "SHOULD FAIL Violation: {{ (index .violations 0).message }}"
  - stage: post-planning
    enabled: false # it's disabled, so it should not be called
    url: some-invalid-url
    testExpression: "{{ .passed }}"
    errorMessageTemplate: "SHOULD FAIL Violation: {{ (index .violations 0).message }}"
`, terraformName, sourceName, server.URL()+"/terraform/admission/pass", server.URL()+"/terraform/admission/fail")), runnerServer.Scheme)
	g.Expect(err).ToNot(HaveOccurred())

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

	It("should be reconciled and contain some status conditions.")
	By("checking that the TF resource's status conditions has some elements.")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return -1
		}
		return len(createdHelloWorldTF.Status.Conditions)
	}, timeout, interval).ShouldNot(BeZero())

	It("should stop by the webhook because it returns passed=false.")
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
					"Status":  c.Status,
				}
			}
		}
		return createdHelloWorldTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    infrav1.ConditionTypePlan,
		"Reason":  "PostPlanningWebhookFailed",
		"Message": "SHOULD FAIL Violation: Max nodes count in GCP in terraform helloworld (1 occurrences)",
		"Status":  metav1.ConditionFalse,
	}))
}
