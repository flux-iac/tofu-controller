package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000311_init_container_test(t *testing.T) {
	Spec("This spec describes the runner pod when it has an Init Container defined.")
	It("should be defined on the Terraform object and appear correctly on the Pod object")

	const (
		sourceName    = "init-container-test-source"
		terraformName = "init-container-test-terraform"
	)
	g := NewWithT(t)
	ctx := context.Background()

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

	Given("the GitRepository's reconciled status.")
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

	By("checking that the status and its URL gets reconciled")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, &createdRepo)).Should(Succeed())

	Given("a Terraform resource with manual approve, attached to the given GitRepository")
	By("creating a new TF resource and attaching to the repo via `sourceRef`, with no .spec.runnerPodTemplate.spec.initContainers specified.")
	helloWorldTF := infrav1.Terraform{}
	err := helloWorldTF.FromBytes([]byte(fmt.Sprintf(`
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: %s
  namespace: flux-system
spec:
  path: ./terraform-hello-world-example
  interval: 10s
  sourceRef:
    kind: GitRepository
    name: %s
    namespace: flux-system
  runnerPodTemplate:
    spec:
      initContainers:
      - name: testing
        image: busybox:latest
        cmd:
        - sleep
        - 5
`, terraformName, sourceName)), runnerServer.Scheme)
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

	It("should run the pod")
	By("checking that there is an init container defined")
	runnerPod := corev1.PodSpec{}
	g.Eventually(func() map[string]string {
		runnerPod = reconciler.runnerPodSpec(createdHelloWorldTF, "tlsSecret")
		return map[string]string{
			"name":  runnerPod.InitContainers[0].Name,
			"image": runnerPod.InitContainers[0].Image,
		}
	}, timeout, interval).Should(Equal(map[string]string{
		"name":  "testing",
		"image": "busybox:latest",
	}))
}

func Test_000311_init_container_delay_all_outputs_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource when not specified a list of output to .spec.writeOutputsToSecret.")
	It("should be reconciled and write all outputs to the output secret.")

	const (
		sourceName          = "init-container-delay-all-output-source"
		terraformName       = "init-container-delay-all-output"
		terraformSecretName = "icdao-output-secret"
	)
	g := NewWithT(t)
	ctx := context.Background()

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
			Digest:         "sha256:80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the status and its URL gets reconciled.")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := &sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, createdRepo)).Should(Succeed())

	Given("a Terraform resource with auto approve, without the list of outputs, attached to the given GitRepository resource")
	By("creating a new TF resource and attaching to the repo via `sourceRef`.")
	By("not specifying the outputs list of .spec.writeOutputsToSecret.")
	helloWorldTF := infrav1.Terraform{}
	err := helloWorldTF.FromBytes([]byte(fmt.Sprintf(`
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: %s
  namespace: flux-system
spec:
  path: ./terraform-hello-world-example
  approvePlan: auto
  interval: 10s
  sourceRef:
    kind: GitRepository
    name: %s
    namespace: flux-system
  writeOutputsToSecret:
    name: %s
  runnerPodTemplate:
    spec:
      initContainers:
      - name: testing
        image: busybox:latest
        cmd:
        - sleep
        - 5
`, terraformName, sourceName, terraformSecretName)), runnerServer.Scheme)
	g.Expect(err).ToNot(HaveOccurred())

	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) }()

	By("checking that the TF resource existed inside the cluster.")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	// We'll need to retry getting this newly created Terraform, Given that creation may not immediately happen.
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	It("should be reconciled and produce the correct output secret.")
	By("checking that the named output secret contains all outputs.")
	outputKey := types.NamespacedName{Namespace: "flux-system", Name: terraformSecretName}
	outputSecret := corev1.Secret{}
	g.Eventually(func() (int, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		if err != nil {
			return -1, err
		}
		return len(outputSecret.Data), nil
	}, timeout, interval).Should(Equal(1))

	By("checking that the output secret contains the correct output data, provisioned by the TF resource.")
	expectedOutputValue := map[string]string{
		"Name":        terraformSecretName,
		"Namespace":   "flux-system",
		"Value":       "Hello, World!",
		"OwnerRef[0]": string(createdHelloWorldTF.UID),
	}
	g.Eventually(func() (map[string]string, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		value := string(outputSecret.Data["hello_world"])
		return map[string]string{
			"Name":        outputSecret.Name,
			"Namespace":   outputSecret.Namespace,
			"Value":       value,
			"OwnerRef[0]": string(outputSecret.OwnerReferences[0].UID),
		}, err
	}, timeout, interval).Should(Equal(expectedOutputValue), "expected output %v", expectedOutputValue)

}
