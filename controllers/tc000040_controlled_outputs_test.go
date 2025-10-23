package controllers

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/gomega"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000040_controlled_outputs_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource when it is controlled the set of outputs.")
	It("should be reconciled and have a specific set of outputs.")

	const (
		sourceName    = "test-tf-controller-controlled-output"
		terraformName = "helloworld-controlled-output"
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
	defer waitResourceToBeDelete(g, &testRepo)

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
		Artifact: &meta.Artifact{
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

	Given("a Terraform resource with auto approve, with controlled output, attached to the given GitRepository resource")
	By("creating a new TF resource and attaching to the repo via `sourceRef`.")
	By("specifying the .spec.writeOutputsToSecret to output only `hello_world` output.")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Interval: metav1.Duration{Duration: time.Second * 10},
			WriteOutputsToSecret: &infrav1.WriteOutputsToSecretSpec{
				Name:        "tf-output-" + terraformName,
				Labels:      map[string]string{"my-new-label": "exists"},
				Annotations: map[string]string{"my-new-annotation": "also-exists"},
				Outputs: []string{
					"hello_world",
				},
			},
		},
	}
	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer waitResourceToBeDelete(g, &helloWorldTF)

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

	It("should be reconciled and produce the correct output secret.")
	By("checking that the named output secret contains a binary data.")
	outputKey := types.NamespacedName{Namespace: "flux-system", Name: "tf-output-" + terraformName}
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
		"Name":           "tf-output-" + terraformName,
		"Namespace":      "flux-system",
		"Value":          "Hello, World!",
		"OwnerRef[0]":    string(createdHelloWorldTF.UID),
		"Labels[0]":      "exists",
		"Annotations[0]": "also-exists",
	}
	g.Eventually(func() (map[string]string, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		value := string(outputSecret.Data["hello_world"])
		return map[string]string{
			"Name":           outputSecret.Name,
			"Namespace":      outputSecret.Namespace,
			"Value":          value,
			"OwnerRef[0]":    string(outputSecret.OwnerReferences[0].UID),
			"Labels[0]":      outputSecret.Labels["my-new-label"],
			"Annotations[0]": outputSecret.Annotations["my-new-annotation"],
		}, err

	}, timeout, interval).Should(Equal(expectedOutputValue), "expected output %v", expectedOutputValue)
}
