package controllers

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	infrav1 "github.com/chanwit/tf-controller/api/v1alpha1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000081_varsfrom_accepts_many_configMaps(t *testing.T) {
	const (
		sourceName    = "src-vars-from-many-config-maps"
		terraformName = "tf-vars-from-many-config-maps"
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
		URL: server.URL() + "/tf-multi-var.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/tf-multi-var.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "52fbbf10455df51136a0c43e0f548c01acdbafca5cbad12c787612e47a4aa815",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the status and its URL gets reconciled")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := &sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, createdRepo)).Should(Succeed())

	By("preparing vars configMaps")
	cmData := []struct {
		name string
		data map[string]string
	}{
		{
			name: "cm1",
			data: map[string]string{
				"cluster_name": "felix",
			},
		},
		{
			name: "cm2",
			data: map[string]string{
				"region":      "eu-west-1",
				"environment": "dev",
			},
		},
	}
	for _, cm := range cmData {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cm.name,
				Namespace: "flux-system",
			},
			Data: cm.data,
		}
		g.Expect(k8sClient.Create(ctx, configMap)).Should(Succeed())
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
			VarsFrom: []infrav1.VarsReference{
				{
					Kind: "ConfigMap",
					Name: "cm1",
				},
				{
					Kind:     "ConfigMap",
					Name:     "cm2",
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
		"Value":       "dev-eu-west-1-felix",
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
