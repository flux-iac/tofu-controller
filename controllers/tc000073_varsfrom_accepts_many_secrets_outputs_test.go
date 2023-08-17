//go:build flaky

package controllers

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000073_varsfrom_accepts_many_secrets(t *testing.T) {
	const (
		sourceName    = "src-vars-from-many-secrets"
		terraformName = "tf-vars-from-many-secrets"
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
			URL:            server.URL() + "/tf-multi-var.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:52fbbf10455df51136a0c43e0f548c01acdbafca5cbad12c787612e47a4aa815",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the status and its URL gets reconciled")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := &sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, createdRepo)).Should(Succeed())

	By("preparing vars configMaps")
	secretData := []struct {
		name string
		data map[string]string
	}{
		{
			name: "s1",
			data: map[string]string{
				"cluster_name": "garfield",
			},
		},
		{
			name: "s2",
			data: map[string]string{
				"region":      "eu-west-1",
				"environment": "dev",
			},
		},
	}
	for _, s := range secretData {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.name,
				Namespace: "flux-system",
			},
			StringData: s.data,
		}
		g.Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
		defer func() { g.Expect(k8sClient.Delete(ctx, secret)).Should(Succeed()) }()
	}

	By("creating a new TF and attaching to the repo")
	testTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./tf-multi-var-with-outputs",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Interval: metav1.Duration{Duration: time.Second * 10},
			VarsFrom: []infrav1.VarsReference{
				{
					Kind: "Secret",
					Name: "s1",
				},
				{
					Kind:     "Secret",
					Name:     "s2",
					VarsKeys: []string{"environment", "region"},
				},
			},
			WriteOutputsToSecret: &infrav1.WriteOutputsToSecretSpec{
				Name: "tf-output-" + terraformName,
				Outputs: []string{
					"cluster_id",
				},
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, &testTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &testTF)).Should(Succeed()) }()

	By("checking that the terraform resource got created")
	testTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	testTFInstance := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, testTFKey, &testTFInstance)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	By("checking that the TF output secret contains binary data")
	outputKey := types.NamespacedName{Namespace: "flux-system", Name: "tf-output-" + terraformName}
	outputSecret := corev1.Secret{}
	g.Eventually(func() (int, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		if err != nil {
			return -1, err
		}
		return len(outputSecret.Data), nil
	}, timeout, interval).Should(Equal(1))

	By("checking that the TF output secret contains the correct output provisioned by the TF resource")
	// Value is a JSON representation of TF's OutputMeta
	expectedOutputValue := map[string]string{
		"Name":        "tf-output-" + terraformName,
		"Namespace":   "flux-system",
		"Value":       "dev-eu-west-1-garfield",
		"OwnerRef[0]": string(testTFInstance.UID),
	}
	g.Eventually(func() (map[string]string, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		value := string(outputSecret.Data["cluster_id"])
		return map[string]string{
			"Name":        outputSecret.Name,
			"Namespace":   outputSecret.Namespace,
			"Value":       value,
			"OwnerRef[0]": string(outputSecret.OwnerReferences[0].UID),
		}, err
	}, timeout, interval).Should(Equal(expectedOutputValue), "expected output %v", expectedOutputValue)

}
