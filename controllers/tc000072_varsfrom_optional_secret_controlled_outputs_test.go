package controllers

import (
	"context"
	infrav1 "github.com/chanwit/tf-controller/api/v1alpha1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000072_varsfrom_optional_secret_and_controlled_outputs_test(t *testing.T) {
	const (
		sourceName    = "tf-vars-from-optional-secret-controlled-output"
		terraformName = "helloworld-vars-from-optional-secret-controlled-output"
	)
	g := NewWithT(t)
	ctx := context.Background()

	by("creating a new Git repository object")
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

	by("setting the git repo status object, the URL, and the correct checksum")
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
		URL: server.URL() + "/env.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/env.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "d021eda9b869586f5a43ad1ba7f21e4bf9b3970443236755463f22824b525316",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	by("checking that the status and its URL gets reconciled")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := &sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, createdRepo)).Should(Succeed())

	by("creating a new TF and attaching to the repo")
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
			// TODO change to a better type
			VarsFrom: &infrav1.VarsReference{
				Kind: "Secret",
				Name: "my-vars-" + terraformName,
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

	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	it("should have an error")
	by("checking that the Ready Status of the TF program reporting the artifact error.")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &helloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range helloWorldTF.Status.Conditions {
			if c.Type == "Ready" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
					"Status":  c.Status,
				}
			}
		}
		return helloWorldTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Reason":  "TFExecPlanFailed",
		"Message": "Secret \"my-vars-helloworld-vars-from-optional-secret-controlled-output\" not found",
		"Status":  metav1.ConditionFalse,
		"Type":    "Ready",
	}))

	helloWorldTF.Spec.VarsFrom.Optional = true
	g.Expect(k8sClient.Update(ctx, &helloWorldTF)).Should(Succeed())

	by("checking that the TF output secret contains a binary data")
	outputKey := types.NamespacedName{Namespace: "flux-system", Name: "tf-output-" + terraformName}
	outputSecret := corev1.Secret{}
	g.Eventually(func() (int, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		if err != nil {
			return -1, err
		}
		return len(outputSecret.Data), nil
	}, timeout, interval).Should(Equal(1))

	by("checking that the TF output secrets contains the correct output provisioned by the TF hello world")
	// Value is a JSON representation of TF's OutputMeta
	expectedOutputValue := map[string]string{
		"Name":        "tf-output-" + terraformName,
		"Namespace":   "flux-system",
		"Value":       "{\"sensitive\":false,\"type\":\"string\",\"value\":\"Hello, World!\"}",
		"OwnerRef[0]": string(helloWorldTF.UID),
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

	by("preparing my-vars secret")
	myVars := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-vars-" + terraformName,
			Namespace: "flux-system",
		},
		Data: map[string][]byte{
			"subject": []byte("my secret cat"),
		},
		Type: corev1.SecretTypeOpaque,
	}
	g.Expect(k8sClient.Create(ctx, &myVars)).Should(Succeed())

	g.Expect(k8sClient.Get(ctx, helloWorldTFKey, &helloWorldTF)).Should(Succeed())
	helloWorldTF.Spec.Force = true
	g.Expect(k8sClient.Update(ctx, &helloWorldTF)).Should(Succeed())

	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &helloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range helloWorldTF.Status.Conditions {
			if c.Type == "Ready" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
					"Status":  c.Status,
				}
			}
		}
		return helloWorldTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Reason":  "TerraformOutputsWritten",
		"Message": "Terraform Outputs Written",
		"Status":  metav1.ConditionTrue,
		"Type":    "Ready",
	}))

	by("checking that the TF output secret contains a binary data")
	g.Eventually(func() (int, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		if err != nil {
			return -1, err
		}
		return len(outputSecret.Data), nil
	}, timeout, interval).Should(Equal(1))

	by("checking that the TF output secrets contains the correct output provisioned by the TF hello world")
	// Value is a JSON representation of TF's OutputMeta
	expectedOutputValue = map[string]string{
		"Name":        "tf-output-" + terraformName,
		"Namespace":   "flux-system",
		"Value":       "{\"sensitive\":false,\"type\":\"string\",\"value\":\"Hello, my secret cat!\"}",
		"OwnerRef[0]": string(helloWorldTF.UID),
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
