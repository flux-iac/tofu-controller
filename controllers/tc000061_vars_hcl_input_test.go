package controllers

import (
	"context"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000061_vars_hcl_input_test(t *testing.T) {
	const (
		sourceName    = "gs-hcl-vars-output"
		terraformName = "tf-hcl-vars-output"
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
		Artifact: &meta.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/tf-hcl-var-with-outputs.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:ff6f6d2a8da451142a4166fa66e5e02b43d2613023587100f24b99c9b5397e9d",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the status and its URL gets reconciled")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := &sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, createdRepo)).Should(Succeed())

	By("creating a new TF and attaching to the repo")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./tf-hcl-var-with-outputs",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Vars: []infrav1.Variable{
				{
					Name: "cluster_spec",
					Value: &apiextensionsv1.JSON{Raw: []byte(`{
						"region": "eu-test-1",
						"env": "stg",
						"cluster": "winter-squirrel",
						"active": true,
						"nodes": 10
					}`)},
				},
				{
					Name:  "zones",
					Value: &apiextensionsv1.JSON{Raw: []byte(`["a","b","c"]`)},
				},
			},
			Interval: metav1.Duration{Duration: time.Second * 10},
			WriteOutputsToSecret: &infrav1.WriteOutputsToSecretSpec{
				Name: "tf-output-" + terraformName,
				Outputs: []string{
					"active",
					"cluster_id",
					"node_count",
					"azs",
				},
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer waitResourceToBeDelete(g, &helloWorldTF)

	By("checking that the hello world TF got created")
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

	By("checking that the TF output secret contains a binary data")
	outputKey := types.NamespacedName{Namespace: "flux-system", Name: "tf-output-" + terraformName}
	outputSecret := corev1.Secret{}
	g.Eventually(func() (int, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		if err != nil {
			return -1, err
		}
		return len(outputSecret.Data), nil
	}, timeout, interval).Should(Equal(7))

	By("checking that the TF output secrets contains the correct output provisioned by the TF hello world")
	// Value is a JSON representation of TF's OutputMeta
	expectedOutputValue := map[string]interface{}{
		"Name":      "tf-output-" + terraformName,
		"Namespace": "flux-system",
		"Values": map[string]string{
			"cluster_id":       "eu-test-1:stg:winter-squirrel",
			"active":           "true",
			"active__type":     `"bool"`,
			"node_count":       "10",
			"node_count__type": `"number"`,
			"azs":              "[\n      \"eu-test-1a\",\n      \"eu-test-1b\",\n      \"eu-test-1c\"\n    ]",
			"azs__type":        "[\n      \"list\",\n      \"string\"\n    ]",
		},
		"OwnerRef[0]": string(createdHelloWorldTF.UID),
	}
	g.Eventually(func() (map[string]interface{}, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		values := map[string]string{
			"cluster_id":       string(outputSecret.Data["cluster_id"]),
			"active":           string(outputSecret.Data["active"]),
			"active__type":     string(outputSecret.Data["active__type"]),
			"node_count":       string(outputSecret.Data["node_count"]),
			"node_count__type": string(outputSecret.Data["node_count__type"]),
			"azs":              string(outputSecret.Data["azs"]),
			"azs__type":        string(outputSecret.Data["azs__type"]),
		}
		return map[string]interface{}{
			"Name":        outputSecret.Name,
			"Namespace":   outputSecret.Namespace,
			"Values":      values,
			"OwnerRef[0]": string(outputSecret.OwnerReferences[0].UID),
		}, err
	}, timeout, interval).Should(Equal(expectedOutputValue), "expected output %v", expectedOutputValue)
}
