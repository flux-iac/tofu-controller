package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000063_values_hcl_test(t *testing.T) {
	const (
		sourceName    = "gs-hcl-values-encoding"
		terraformName = "tf-hcl-values-encoding"

		clusterName = "my-cluster"
		accessKey   = "my-access-key"
		secretKey   = "my-secret-key"
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
	defer waitResourceToBeDelete(g, &testRepo)

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
    secret_key: my-secret-key
  path: ./tf-hcl-values-advanced-example
  sourceRef:
    kind: GitRepository
    name: %s
    namespace: flux-system
  interval: 10s
  approvePlan: auto
  writeOutputsToSecret:
    name: tf-output-%s
    outputs:
    - values_json_blob
`, terraformName, sourceName, terraformName)), runnerServer.Scheme)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer waitResourceToBeDelete(g, &helloWorldTF)

	By("checking that the hello world TF got created")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	// We'll need to retry getting this newly created Terraform, Given that creation may not immediately happen.
	g.Eventually(func() bool {
		_ = k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		for _, c := range *createdHelloWorldTF.GetStatusConditions() {
			if c.Type == "Ready" && c.Status == metav1.ConditionTrue {
				return true
			}
			fmt.Println(c.Message)
		}
		return false
	}, timeout, interval).Should(BeTrue())

	By("checking that the TF output secret contains a binary data")
	outputKey := types.NamespacedName{Namespace: "flux-system", Name: "tf-output-" + terraformName}
	outputSecret := corev1.Secret{}
	g.Eventually(func() int {
		_ = k8sClient.Get(ctx, outputKey, &outputSecret)
		return len(outputSecret.Data)
	}, timeout, interval).Should(Equal(1))

	By("checking that the TF output secrets contains the correct output provisioned by the TF hello world")
	// Value is a JSON representation of TF's OutputMeta
	expectedOutputValue := map[string]interface{}{
		"Name":      "tf-output-" + terraformName,
		"Namespace": "flux-system",
		"Values": map[string]string{
			"values_json_blob": fmt.Sprintf(`{"Values_AccessKey":"%s","Values_Cluster":"%s","Values_SecretKey":"%s"}`, accessKey, clusterName, secretKey),
		},
	}
	g.Eventually(func() (map[string]interface{}, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		values := map[string]string{
			"values_json_blob": string(outputSecret.Data["values_json_blob"]),
		}
		return map[string]interface{}{
			"Name":      outputSecret.Name,
			"Namespace": outputSecret.Namespace,
			"Values":    values,
		}, err
	}, timeout, interval).Should(Equal(expectedOutputValue), "expected output %v", expectedOutputValue)
}
