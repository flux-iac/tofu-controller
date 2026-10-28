package controllers

import (
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	. "github.com/onsi/gomega"

	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000012_src_externalartifact_no_outputs_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource which is stored in an ExternalArtifact. There is no backend and auto-approve is enabled.")
	It("should be reconciled, and re-reconciled when the ExternalArtifact's revision changes.")

	var (
		sourceName    = "test-tf-controller-src-externalartifact-no-output"
		terraformName = "src-externalartifact-helloworld-no-outputs-" + rand.String(6)
	)
	g := NewWithT(t)
	ctx := t.Context()

	Given("an ExternalArtifact")
	By("defining a new ExternalArtifact resource, as produced by an ArtifactGenerator.")
	testExternalArtifact := sourcev1.ExternalArtifact{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceName,
			Namespace: "flux-system",
		},
	}

	By("creating the ExternalArtifact resource in the cluster.")
	It("should be created successfully.")
	g.Expect(k8sClient.Create(ctx, &testExternalArtifact)).Should(Succeed())
	defer waitResourceToBeDelete(g, &testExternalArtifact)

	Given("the ExternalArtifact's reconciled status.")
	By("setting the ExternalArtifact's status, with the downloadable BLOB's URL, and the correct checksum.")
	updatedTime := time.Now()
	testExternalArtifact.Status = sourcev1.ExternalArtifactStatus{
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "Succeeded",
				Message:            "reconciliation succeeded",
			},
		},
		Artifact: &meta.Artifact{
			Path:           "externalartifact/flux-system/" + sourceName + "/80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "latest@sha256:80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406",
			Digest:         "sha256:80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testExternalArtifact)).Should(Succeed())

	Given("a Terraform resource with auto approve, attached to the given ExternalArtifact resource.")
	By("creating a new TF resource and attaching to the artifact via `sourceRef`, with an interval long enough that only a source change triggers a new reconciliation.")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      sourcev1.ExternalArtifactKind,
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Interval: metav1.Duration{Duration: time.Hour},
		},
	}
	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer waitResourceToBeDelete(g, &helloWorldTF)

	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	lastApplied := func() map[string]any {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]any{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"LastAppliedPlan": createdHelloWorldTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}

	It("should be applied with the plan named after the artifact revision.")
	By("checking that the reason of the Apply condition is TerraformAppliedSucceed, and the LastAppliedPlan is plan-latest-80ddfd18eb.")
	g.Eventually(lastApplied, timeout, interval).Should(Equal(map[string]any{
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"LastAppliedPlan": "plan-latest-80ddfd18eb",
	}))

	Given("a new revision of the ExternalArtifact.")
	By("updating the ExternalArtifact's status to point at a new artifact.")
	updatedTime = time.Now()
	testExternalArtifact.Status.Artifact = &meta.Artifact{
		Path:           "externalartifact/flux-system/" + sourceName + "/525802635a47a5ae3f9c058a2b958aac0daef08efbe100a4fc16833df5201b94.tar.gz",
		URL:            server.URL() + "/2222.tar.gz",
		Revision:       "latest@sha256:525802635a47a5ae3f9c058a2b958aac0daef08efbe100a4fc16833df5201b94",
		Digest:         "sha256:525802635a47a5ae3f9c058a2b958aac0daef08efbe100a4fc16833df5201b94",
		LastUpdateTime: metav1.Time{Time: updatedTime},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testExternalArtifact)).Should(Succeed())

	It("should be re-applied with the new revision, before its interval elapses.")
	By("checking that the LastAppliedPlan becomes plan-latest-525802635a.")
	g.Eventually(lastApplied, timeout, interval).Should(Equal(map[string]any{
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"LastAppliedPlan": "plan-latest-525802635a",
	}))
}
