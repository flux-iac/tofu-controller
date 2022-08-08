package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"
	. "github.com/onsi/gomega"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000062_vars_hcl_test(t *testing.T) {
	const (
		sourceName     = "gs-hcl-vars-encoding"
		terraformName  = "tf-hcl-vars-encoding"
		varsSecretName = "sc-hcl-vars-encoding"
		clusterName    = "test-vars-cluster"
		accessKey      = "1234567890"
		secretKey      = "abcdefghij"
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
			Interval:          metav1.Duration{Duration: time.Second * 30},
			GitImplementation: "go-git",
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
		URL: server.URL() + "/tf-hcl-vars-advanced-example.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/tf-hcl-vars-advanced-example.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "d8dceaa02d091cbb699795696943af856a2a928d8fb5328a4299dbeb43f9528f",
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

	By("creating a secret to hold variables")
	varsSecretKey := types.NamespacedName{Namespace: "flux-system", Name: varsSecretName}
	varsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      varsSecretKey.Name,
			Namespace: varsSecretKey.Namespace,
		},
		StringData: map[string]string{
			"access_key": accessKey,
			"secret_key": secretKey,
		},
	}
	g.Expect(k8sClient.Create(ctx, &varsSecret)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &varsSecret)).Should(Succeed()) }()

	By("creating a new TF and attaching to the repo")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./tf-hcl-vars-advanced-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Vars: []infrav1.Variable{
				{
					Name:  "cluster_name",
					Value: utils.MustJSONEncodeBytes(t, []byte(clusterName)),
				},
			},
			VarsFrom: []infrav1.VarsReference{
				{
					Kind: "Secret",
					Name: varsSecretName,
				},
			},
			Interval: metav1.Duration{Duration: time.Second * 10},
			WriteOutputsToSecret: &infrav1.WriteOutputsToSecretSpec{
				Name: "tf-output-" + terraformName,
				Outputs: []string{
					"json_blob",
				},
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) }()

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
			"json_blob": fmt.Sprintf(`{"AccessKey":"%s","Cluster":"%s","SecretKey":"%s"}`, accessKey, clusterName, secretKey),
		},
	}
	g.Eventually(func() (map[string]interface{}, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		values := map[string]string{
			"json_blob": string(outputSecret.Data["json_blob"]),
		}
		return map[string]interface{}{
			"Name":      outputSecret.Name,
			"Namespace": outputSecret.Namespace,
			"Values":    values,
		}, err
	}, timeout, interval).Should(Equal(expectedOutputValue), "expected output %v", expectedOutputValue)
}
