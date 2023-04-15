package controllers

import (
	. "github.com/onsi/gomega"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestObjectEncode(t *testing.T) {
	g := NewGomegaWithT(t)

	helloWorldTF := infrav1.Terraform{
		TypeMeta: metav1.TypeMeta{
			APIVersion: infrav1.GroupVersion.Group + "/" + infrav1.GroupVersion.Version,
			Kind:       infrav1.TerraformKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tf-1",
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      "flux-system",
				Namespace: "flux-system",
			},
		},
	}

	b, err := helloWorldTF.ToBytes(reconciler.Scheme)
	g.Expect(err).To(BeNil())
	var tf infrav1.Terraform
	err = tf.FromBytes(b, runnerServer.Scheme)
	g.Expect(err).To(BeNil())
	g.Expect(tf).To(Equal(helloWorldTF))
	g.Expect(tf.Spec.ApprovePlan).To(Equal("auto"))
	g.Expect(tf.Spec.Path).To(Equal("./terraform-hello-world-example"))
}
