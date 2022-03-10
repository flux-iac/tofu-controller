package controllers

import (
	"testing"

	. "github.com/onsi/gomega"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000260_runner_pod_test(t *testing.T) {
	Spec("This spec describes a runner pod creation process")

	const (
		terraformName      = "runner-pod-test"
		sourceName         = "runner-pod-test"
		serviceAccountName = "helloworld-tf-runner"
	)

	g := NewWithT(t)

	It("shoud reconcile a runner pod")
	By("pass a terraform object, the runner pod spec should be accurate")
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
			ServiceAccountName: serviceAccountName,
		},
	}

	spec := reconciler.runnerPodSpec(helloWorldTF)
	g.Expect(spec.ServiceAccountName == serviceAccountName)
}
