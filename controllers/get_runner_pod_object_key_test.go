package controllers

import (
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"

	. "github.com/onsi/gomega"
)

func TestGetRunnerPodObjectKey(t *testing.T) {
	g := NewWithT(t)
	tf := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "flux-system",
			Name:      "test",
		},
	}

	result := getRunnerPodObjectKey(tf)
	g.Expect(result).To(Equal(types.NamespacedName{
		Namespace: "flux-system",
		Name:      "test-tf-runner",
	}))
}
