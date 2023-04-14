package controllers

import (
	"context"
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000071_varsfrom_secret_with_varkeys_and_controlled_outputs_test(t *testing.T) {
	const (
		sourceName    = "tf-vars-from-with-varkeys-controlled-output"
		terraformName = "helloworld-vars-from-with-varkeys-controlled-output"
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
			URL:            server.URL() + "/env.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:d021eda9b869586f5a43ad1ba7f21e4bf9b3970443236755463f22824b525316",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the status and its URL gets reconciled")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := &sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, createdRepo)).Should(Succeed())

	By("preparing my-vars secret")
	myVars := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-vars-" + terraformName,
			Namespace: "flux-system",
		},
		Data: map[string][]byte{
			"subject": []byte("my secret cat"),
			"unused":  []byte("unused value"),
		},
		Type: corev1.SecretTypeOpaque,
	}
	g.Expect(k8sClient.Create(ctx, &myVars)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &myVars)).Should(Succeed()) }()

	By("creating a new TF and attaching to the repo")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./terraform-hello-env",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Interval: metav1.Duration{Duration: time.Second * 10},
			// TODO change to a better type
			VarsFrom: []infrav1.VarsReference{
				{
					Kind:     "Secret",
					Name:     "my-vars-" + terraformName,
					VarsKeys: []string{"subject"},
				},
			},
			WriteOutputsToSecret: &infrav1.WriteOutputsToSecretSpec{
				Name: "tf-output-" + terraformName,
				Outputs: []string{
					"hello_world",
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
	}, timeout, interval).Should(Equal(1))

	By("checking that the TF output secrets contains the correct output provisioned By the TF hello world")
	// Value is a JSON representation of TF's OutputMeta
	expectedOutputValue := map[string]string{
		"Name":        "tf-output-" + terraformName,
		"Namespace":   "flux-system",
		"Value":       "Hello, my secret cat!",
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
