package branchplanner

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	gom "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/flux-iac/tofu-controller/api/plan"
	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/internal/config"
	"github.com/flux-iac/tofu-controller/internal/git/provider"
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
				plan.TFPlanNameLabel:      plan.SafeLabelValue("helloworld"),
				plan.TFPlanWorkspaceLabel: plan.SafeLabelValue("default"),
			},
			Annotations: map[string]string{
				plan.TFPlanSavedAnnotation:         "plan-main-1",
				plan.TFPlanFullWorkspaceAnnotation: "default",
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

// TestInformerReadsProviderSecretJustInTime verifies that the informer
// resolves the git provider per PR interaction, re-reading the provider
// secret each time, so that a rotated token (e.g. a short-lived GitHub App
// installation token) is picked up without a restart.
func TestInformerReadsProviderSecretJustInTime(t *testing.T) {
	g := gom.NewWithT(t)
	ns := newNamespace(t, g)
	ctx := t.Context()

	// Create a source for the Terraform object to point to
	source := &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jit-source",
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

	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jit-helloworld",
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
			Name:      "tfplan-default-jit-helloworld",
			Namespace: ns.Name,
			Labels: map[string]string{
				plan.TFPlanNameLabel:      plan.SafeLabelValue("jit-helloworld"),
				plan.TFPlanWorkspaceLabel: plan.SafeLabelValue("default"),
			},
			Annotations: map[string]string{
				plan.TFPlanSavedAnnotation:         "plan-main-1",
				plan.TFPlanFullWorkspaceAnnotation: "default",
			},
		},
		Data: map[string]string{
			"tfplan": "terraform plan output",
		},
	}
	expectToSucceed(t, g, k8sClient.Create(t.Context(), tfOutputCM))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jit-provider-secret",
			Namespace: ns.Name,
		},
		Data: map[string][]byte{
			"token": []byte("token-1"),
		},
	}
	expectToSucceed(t, g, k8sClient.Create(t.Context(), secret))

	dynamicClient, err := dynamic.NewForConfig(k8sConfig)
	g.Expect(err).NotTo(gom.HaveOccurred())

	// A single shared fake provider accumulates call counts across events.
	// The parser fn applies each interaction's options to it, so SetToken
	// records the token used per event.
	fakeProvider := &providerfakes.FakeProvider{}
	parserFn := func(repoURL string, options ...provider.ProviderOption) (provider.Provider, provider.Repository, error) {
		repo, err := provider.RepoFromURL(repoURL)
		if err != nil {
			return nil, provider.Repository{}, err
		}
		for _, opt := range options {
			if err := opt(fakeProvider); err != nil {
				return nil, provider.Repository{}, err
			}
		}
		return fakeProvider, repo, nil
	}

	sharedInformer := createSharedInformer(g, ctx, k8sClient, dynamicClient)

	log := logr.Discard()

	informer, err := NewInformer(
		WithLogger(log),
		WithClusterClient(k8sClient),
		WithProviderOptsFn(func(ctx context.Context) ([]provider.ProviderOption, error) {
			liveSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), liveSecret); err != nil {
				return nil, err
			}
			return provider.OptsFromSecret(liveSecret.Data)
		}),
		WithCustomProviderURLParserFn(parserFn),
		WithSharedInformer(sharedInformer),
	)
	g.Expect(err).NotTo(gom.HaveOccurred())

	go func() {
		g.Expect(informer.Start(ctx)).To(gom.Succeed())
	}()

	g.Eventually(func() bool {
		return informer.HasSynced()
	}).Should(gom.BeTrue())

	// Event 1: patch status to trigger the informer update function.
	expectToSucceed(t, g, k8sClient.Get(ctx, client.ObjectKeyFromObject(tf), tf))
	patch := client.MergeFrom(tf.DeepCopy())
	tf.Status.LastPlanAt = &metav1.Time{Time: time.Now()}
	expectToSucceed(t, g, k8sClient.Status().Patch(ctx, tf, patch))

	g.Eventually(fakeProvider.AddCommentToPullRequestCallCount).Should(gom.Equal(1))

	tokenType, token := fakeProvider.SetTokenArgsForCall(0)
	g.Expect(tokenType).To(gom.Equal(provider.APITokenType))
	g.Expect(token).To(gom.Equal("token-1"))

	// Rotate the token in the secret.
	expectToSucceed(t, g, k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret))
	secret.Data["token"] = []byte("token-2")
	expectToSucceed(t, g, k8sClient.Update(ctx, secret))

	// Event 2: patch status with a strictly later plan time.
	expectToSucceed(t, g, k8sClient.Get(ctx, client.ObjectKeyFromObject(tf), tf))
	patch = client.MergeFrom(tf.DeepCopy())
	tf.Status.LastPlanAt = &metav1.Time{Time: time.Now().Add(time.Hour)}
	expectToSucceed(t, g, k8sClient.Status().Patch(ctx, tf, patch))

	g.Eventually(fakeProvider.AddCommentToPullRequestCallCount).Should(gom.Equal(2))

	tokenType, token = fakeProvider.SetTokenArgsForCall(1)
	g.Expect(tokenType).To(gom.Equal(provider.APITokenType))
	g.Expect(token).To(gom.Equal("token-2"))
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
