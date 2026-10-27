package controllers

import (
	"testing"

	"github.com/flux-iac/tofu-controller/api/plan"
	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTF(name string) *infrav1.Terraform {
	return &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "flux-system",
			Name:      name,
		},
	}
}

func TestSetPlanArtifactRef_Primary(t *testing.T) {
	g := NewWithT(t)
	tf := newTF("hello")

	setPlanArtifactRef(tf, []*runner.SavedObject{
		{ApiVersion: "v1", Kind: "Secret", Namespace: "flux-system", Name: "tfplan-default-hello"},
	}, "plan-abc")

	g.Expect(tf.Status.ArtifactRefs).NotTo(BeNil())
	g.Expect(tf.Status.ArtifactRefs.Plan).NotTo(BeNil())

	locator := tf.Status.ArtifactRefs.Plan.Locator
	g.Expect(locator.Type).To(Equal(infrav1.LocatorKubernetes))
	g.Expect(locator.Kubernetes.APIVersion).To(Equal("v1"))
	g.Expect(locator.Kubernetes.Kind).To(Equal("Secret"))
	g.Expect(locator.Kubernetes.Namespace).To(Equal("flux-system"))
	g.Expect(locator.Kubernetes.Name).To(Equal("tfplan-default-hello"))
	g.Expect(locator.Kubernetes.Key).To(Equal(plan.TFPlanName))

	g.Expect(tf.Status.ArtifactRefs.Plan.PlanID).To(Equal("plan-abc"))
	g.Expect(tf.Status.ArtifactRefs.Plan.Chunks).To(Equal(int32(1)))
	g.Expect(tf.Status.ArtifactRefs.Plan.Compression).To(Equal("gzip"))
}

func TestSetPlanArtifactRef_Chunked(t *testing.T) {
	g := NewWithT(t)
	tf := newTF("hello")

	setPlanArtifactRef(tf, []*runner.SavedObject{
		{ApiVersion: "v1", Kind: "Secret", Namespace: "flux-system", Name: "tfplan-default-hello-0"},
		{ApiVersion: "v1", Kind: "Secret", Namespace: "flux-system", Name: "tfplan-default-hello-1"},
		{ApiVersion: "v1", Kind: "Secret", Namespace: "flux-system", Name: "tfplan-default-hello-2"},
	}, "plan-xyz")

	g.Expect(tf.Status.ArtifactRefs.Plan.Locator.Kubernetes.Name).To(Equal("tfplan-default-hello-0"),
		"the locator must address chunk 0")
	g.Expect(tf.Status.ArtifactRefs.Plan.Chunks).To(Equal(int32(3)))
}

func TestSetPlanArtifactRef_ReplacesPreviousRef(t *testing.T) {
	g := NewWithT(t)
	tf := newTF("hello")
	tf.Status.ArtifactRefs = &infrav1.ArtifactReferences{
		Plan: &infrav1.PlanArtifact{
			Locator: infrav1.SecretLocator("flux-system", "tfplan-default-hello-0", plan.TFPlanName),
			PlanID:  "plan-old",
			Chunks:  3,
		},
	}

	setPlanArtifactRef(tf, []*runner.SavedObject{
		{ApiVersion: "v1", Kind: "Secret", Namespace: "flux-system", Name: "tfplan-default-hello"},
	}, "plan-new")

	g.Expect(tf.Status.ArtifactRefs.Plan.Locator.Kubernetes.Name).To(Equal("tfplan-default-hello"))
	g.Expect(tf.Status.ArtifactRefs.Plan.PlanID).To(Equal("plan-new"))
	g.Expect(tf.Status.ArtifactRefs.Plan.Chunks).To(Equal(int32(1)), "stale chunk count must not survive")
}

func TestSetPlanArtifactRef_EmptyClears(t *testing.T) {
	g := NewWithT(t)
	tf := newTF("hello")
	tf.Status.ArtifactRefs = &infrav1.ArtifactReferences{
		Plan: &infrav1.PlanArtifact{PlanID: "stale"},
	}

	setPlanArtifactRef(tf, nil, "ignored")

	g.Expect(tf.Status.ArtifactRefs.Plan).To(BeNil())
}

func TestClearPlanArtifactRef_NoStatus(t *testing.T) {
	g := NewWithT(t)
	tf := newTF("hello")

	g.Expect(func() { clearPlanArtifactRef(tf) }).NotTo(Panic())
	g.Expect(tf.Status.ArtifactRefs).To(BeNil())
}

func TestSetOutputsArtifactRef(t *testing.T) {
	g := NewWithT(t)
	tf := newTF("hello")

	setOutputsArtifactRef(tf, "hello-outputs")

	g.Expect(tf.Status.ArtifactRefs.Outputs).NotTo(BeNil())

	locator := tf.Status.ArtifactRefs.Outputs.Locator
	g.Expect(locator.Type).To(Equal(infrav1.LocatorKubernetes))
	g.Expect(locator.Kubernetes.APIVersion).To(Equal("v1"))
	g.Expect(locator.Kubernetes.Kind).To(Equal("Secret"))
	g.Expect(locator.Kubernetes.Namespace).To(Equal("flux-system"))
	g.Expect(locator.Kubernetes.Name).To(Equal("hello-outputs"))
	g.Expect(locator.Kubernetes.Key).To(BeEmpty(),
		"the outputs Secret holds one entry per output, so no single key applies")
}

func TestClearOutputsArtifactRef(t *testing.T) {
	g := NewWithT(t)
	tf := newTF("hello")
	tf.Status.ArtifactRefs = &infrav1.ArtifactReferences{
		Outputs: &infrav1.OutputsArtifact{
			Locator: infrav1.SecretLocator("flux-system", "stale", ""),
		},
	}

	clearOutputsArtifactRef(tf)

	g.Expect(tf.Status.ArtifactRefs.Outputs).To(BeNil())
}

func TestSetStateArtifactRef(t *testing.T) {
	g := NewWithT(t)
	tf := newTF("hello")

	setStateArtifactRef(tf, infrav1.SecretLocator("flux-system", "tfstate-default-hello", tfStateSecretKey))

	g.Expect(tf.Status.ArtifactRefs.State).NotTo(BeNil())
	g.Expect(tf.Status.ArtifactRefs.State.Locator.Kubernetes.Name).To(Equal("tfstate-default-hello"))
}

// String is used in logs and events, so it must tolerate status read back from
// the API with a payload that does not match its type.
func TestArtifactLocatorString_ToleratesMissingPayload(t *testing.T) {
	g := NewWithT(t)

	for _, locator := range []infrav1.ArtifactLocator{
		{Type: infrav1.LocatorKubernetes},
		{Type: infrav1.LocatorExternalCluster},
		{Type: infrav1.LocatorExternal},
		{Type: infrav1.LocatorEphemeral},
		infrav1.NoneLocator(),
	} {
		g.Expect(func() { _ = locator.String() }).NotTo(Panic())
		g.Expect(locator.String()).NotTo(BeEmpty())
	}

	g.Expect(infrav1.SecretLocator("flux-system", "tfplan-default-hello", "tfplan").String()).
		To(Equal("Secret flux-system/tfplan-default-hello[tfplan]"))
}
