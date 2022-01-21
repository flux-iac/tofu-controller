package controllers

import (
	"context"
	"testing"
	"time"

	infrav1 "github.com/chanwit/tf-controller/api/v1alpha1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000241_auto_approve_with_health_checks_test(t *testing.T) {
	Spec("This spec describes behaviour when health checks are specified")

	const (
		sourceName    = "health-heck-auto-approve"
		terraformName = "tf-health-heck-auto-approve"
	)

	g := NewWithT(t)
	ctx := context.Background()
	updatedTime := time.Now()

	It("should do health checks")
	By("creating a new Git repository object")
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
		URL: server.URL() + "/file.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the status and its URL gets reconciled")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, &createdRepo)).Should(Succeed())

	By("creating a new TF and attaching to the repo, with approve plan set to auto and drift detection disabled")
	testEnvKubeConfigPath, err := findKubeConfig(testEnv)
	g.Expect(err).Should(BeNil())

	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			Interval:    metav1.Duration{Duration: time.Second * 5},
			ApprovePlan: "auto",
			Path:        "./terraform-hello-world-example",
			BackendConfig: &infrav1.BackendConfigSpec{
				SecretSuffix:    terraformName,
				InClusterConfig: false,
				ConfigPath:      testEnvKubeConfigPath,
			},
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			HealthChecks: []infrav1.HealthCheck{
				{
					Name: "tcpTest",
					URL:  "weave.works:80",
					Type: "tcp",
				},
				{
					Name:    "httpTest",
					URL:     "https://httpbin.org/get",
					Type:    "http",
					Timeout: &metav1.Duration{Duration: time.Second * 5},
				},
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())

	By("checking that the hello world TF gets created")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	By("checking that the plan was applied successfully")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"Message":         c.Message,
					"LastAppliedPlan": createdHelloWorldTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            "Apply",
		"Reason":          "TerraformAppliedSucceed",
		"Message":         "Applied successfully",
		"LastAppliedPlan": "plan-master-b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
	}))

	By("checking that we have outputs available in the TF object")
	g.Eventually(func() []string {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		return createdHelloWorldTF.Status.AvailableOutputs
	}, timeout, interval).Should(Equal([]string{"hello_world"}))

	By("checking that the health checks are performed successfully")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "HealthCheck" {
				return map[string]interface{}{
					"Type":              c.Type,
					"Reason":            c.Reason,
					"Message":           c.Message,
					"HealthCheckStatus": createdHelloWorldTF.Status.HealthCheck.Succeeded,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":              "HealthCheck",
		"Reason":            "HealthCheckSucceed",
		"Message":           "Health checks succeeded",
		"HealthCheckStatus": true,
	}))
}
