package bbp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	tfv1alpha2 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/internal/git/provider/providerfakes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

func TestInformer(t *testing.T) {
	g := NewWithT(t)
	ns := newNamespace(g)
	ctx := context.Background()

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
	expectToSucceed(g, k8sClient.Create(context.TODO(), source))

	// Create a Terraform object to be the template.
	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "helloworld",
			Namespace: ns.Name,
			Labels: map[string]string{
				"infra.weave.works/branch-based-planner": "true",
				"infra.weave.works/pr-id":                "1",
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
	expectToSucceed(g, k8sClient.Create(context.TODO(), tf))

	tfOutputCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tfplan-default-helloworld",
			Namespace: ns.Name,
		},
		Data: map[string]string{
			"tfplan": "terraform plan output",
		},
	}
	expectToSucceed(g, k8sClient.Create(context.TODO(), tfOutputCM))

	dynamicClient, err := dynamic.NewForConfig(k8sConfig)
	g.Expect(err).NotTo(HaveOccurred())

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
	g.Expect(err).NotTo(HaveOccurred())

	go func() {
		g.Expect(informer.Start(ctx)).To(Succeed())
	}()

	g.Eventually(func() bool {
		return informer.HasSynced()
	}).Should(BeTrue())

	// Patch status to trigger informer update function
	k8sClient.Get(ctx, client.ObjectKeyFromObject(tf), tf)
	patch := client.MergeFrom(tf.DeepCopy())
	tf.Status.LastPlanAt = &metav1.Time{Time: time.Now()}
	expectToSucceed(g, k8sClient.Status().Patch(ctx, tf, patch))

	g.Eventually(func() int {
		return gitProvider.AddCommentToPullRequestCallCount()
	}).Should(Equal(1))

	g.Eventually(func() string {
		_, _, body := gitProvider.AddCommentToPullRequestArgsForCall(0)
		return string(body)
	}).Should(ContainSubstring("terraform plan output"))
}

func createSharedInformer(g *WithT, ctx context.Context, client client.Client, dynamicClient dynamic.Interface) cache.SharedIndexInformer {
	restMapper := client.RESTMapper()
	mapping, err := restMapper.RESTMapping(tfv1alpha2.GroupVersion.WithKind(tfv1alpha2.TerraformKind).GroupKind())
	g.Expect(err).NotTo(HaveOccurred())

	tweakListOptionsFunc := func(options *metav1.ListOptions) {
		options.LabelSelector = fmt.Sprintf("%s=%s", LabelKey, LabelValue)
	}

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, time.Minute, corev1.NamespaceAll, tweakListOptionsFunc)

	return factory.ForResource(mapping.Resource).Informer()
}
