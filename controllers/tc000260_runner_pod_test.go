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
		runnerPodImage     = "ghcr.io/weaveworks/tf-runner:test"
	)

	var stringMap = map[string]string{
		"company.com/abc": "xyz",
		"company.com/xyz": "abc",
	}

	g := NewWithT(t)

	It("generate a runner pod template")
	By("passing a terraform object, the runner pod template should be accurate")
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
			RunnerPodTemplate: infrav1.RunnerPodTemplate{
				Metadata: infrav1.RunnerPodMetadata{
					Labels:      stringMap,
					Annotations: stringMap,
				},
				Spec: infrav1.RunnerPodSpec{
					Image: runnerPodImage,
				},
			},
		},
	}

	spec := reconciler.runnerPodSpec(helloWorldTF)
	g.Expect(spec.ServiceAccountName == serviceAccountName)
	g.Expect(spec.Containers[0].Image == runnerPodImage)

	podTemplate := runnerPodTemplate(helloWorldTF)
	g.Expect(func() bool {
		for k, v := range stringMap {
			if v != podTemplate.ObjectMeta.Labels[k] {
				return false
			}
		}
		for k, v := range stringMap {
			if v != podTemplate.ObjectMeta.Annotations[k] {
				return false
			}
		}
		return true
	})
}
