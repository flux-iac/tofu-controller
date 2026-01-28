package branchplanner

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	gom "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/internal/config"
	"github.com/flux-iac/tofu-controller/internal/git/provider/providerfakes"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

func TestInformer(t *testing.T) {
	g := gom.NewWithT(t)
	ns := newNamespace(t, g)
	ctx := t.Context()

	// Create a source for the Terraform object to point to
	source := &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "original-source",
			Namespace: ns.Name,
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/tf-controller/helloworld",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "main",
			},
		},
	}
	expectToSucceed(t, g, k8sClient.Create(t.Context(), source))

	// Create a Terraform object to be the template.
	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "helloworld",
			Namespace: ns.Name,
			Labels: map[string]string{
				config.LabelKey:           config.LabelValue,
				"infra.weave.works/pr-id": "1",
			},
		},
		Spec: infrav1.TerraformSpec{
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Name:      source.Name,
				Namespace: ns.Name,
				Kind:      "GitRepository",
			},
		},
	}
	expectToSucceed(t, g, k8sClient.Create(t.Context(), tf))

	tfOutputCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tfplan-default-helloworld",
			Namespace: ns.Name,
			Labels: map[string]string{
				"infra.contrib.fluxcd.io/plan-name":      "helloworld",
				"infra.contrib.fluxcd.io/plan-workspace": "default",
			},
			Annotations: map[string]string{
				"savedPlan": "plan-main-1",
			},
		},
		Data: map[string]string{
			"tfplan": "terraform plan output",
		},
	}
	expectToSucceed(t, g, k8sClient.Create(t.Context(), tfOutputCM))

	dynamicClient, err := dynamic.NewForConfig(k8sConfig)
	g.Expect(err).NotTo(gom.HaveOccurred())

	gitProvider := &providerfakes.FakeProvider{}

	sharedInformer := createSharedInformer(g, ctx, k8sClient, dynamicClient)

	// since the informer UpdateFunc does not return any errors, the best way to debug issues there is using a logger
	// uncomment this out to debug informer issues
	// log := logger.NewLogger(logger.Options{})
	// logger.SetLogger(log)
	log := logr.Discard()

	informer, err := NewInformer(
		WithLogger(log),
		WithClusterClient(k8sClient),
		WithGitProvider(gitProvider),
		WithSharedInformer(sharedInformer),
	)
	g.Expect(err).NotTo(gom.HaveOccurred())

	go func() {
		g.Expect(informer.Start(ctx)).To(gom.Succeed())
	}()

	g.Eventually(func() bool {
		return informer.HasSynced()
	}).Should(gom.BeTrue())

	// Patch status to trigger informer update function
	k8sClient.Get(ctx, client.ObjectKeyFromObject(tf), tf)
	patch := client.MergeFrom(tf.DeepCopy())
	tf.Status.LastPlanAt = &metav1.Time{Time: time.Now()}
	expectToSucceed(t, g, k8sClient.Status().Patch(ctx, tf, patch))

	g.Eventually(func() int {
		return gitProvider.AddCommentToPullRequestCallCount()
	}).Should(gom.Equal(1))

	g.Eventually(func() string {
		_, _, body := gitProvider.AddCommentToPullRequestArgsForCall(0)
		return string(body)
	}).Should(gom.ContainSubstring("terraform plan output"))
}

func createSharedInformer(g *gom.WithT, ctx context.Context, client client.Client, dynamicClient dynamic.Interface) cache.SharedIndexInformer {
	restMapper := client.RESTMapper()
	mapping, err := restMapper.RESTMapping(infrav1.GroupVersion.WithKind(infrav1.TerraformKind).GroupKind())
	g.Expect(err).NotTo(gom.HaveOccurred())

	tweakListOptionsFunc := func(options *metav1.ListOptions) {
		options.LabelSelector = fmt.Sprintf("%s=%s", config.LabelKey, config.LabelValue)
	}

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, time.Minute, corev1.NamespaceAll, tweakListOptionsFunc)

	return factory.ForResource(mapping.Resource).Informer()
}
