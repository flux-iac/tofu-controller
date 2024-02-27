package polling_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/internal/config"
	"github.com/flux-iac/tofu-controller/internal/git/provider"
	"github.com/flux-iac/tofu-controller/internal/git/provider/providerfakes"
	"github.com/flux-iac/tofu-controller/internal/server/polling"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/logger"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	logLevel           = "error"
	eventuallyTimeout  = time.Second * 30
	eventuallyInterval = time.Millisecond * 500
)

func Test_Server(t *testing.T) {
	g := gomega.NewWithT(t)
	objects := testResources(config.DefaultNamespace)
	fakeClient := fake.NewClientBuilder().WithObjects(objects...).WithStatusSubresource(objects...).Build()
	log := logger.NewLogger(logger.Options{LogLevel: logLevel}).WithName("informer")

	fakePRConf := struct{ closed bool }{closed: false}
	fakeComments := []*provider.Comment{}

	fakeProvider := providerfakes.FakeProvider{
		ListPullRequestsStub: func(ctx context.Context, repo provider.Repository) ([]provider.PullRequest, error) {
			return []provider.PullRequest{
				{
					Repository: repo,
					Number:     1,
					BaseBranch: "main",
					HeadBranch: "patch-1",
					BaseSha:    "2861800e346d71bf74eac623387e1b2b507ef4af",
					HeadSha:    "ae22c1b3dad69da20a4a02cd090ac9f6183babea",
					Closed:     fakePRConf.closed,
				},
			}, nil
		},
		GetLastCommentsStub: func(context.Context, provider.PullRequest, time.Time) ([]*provider.Comment, error) {
			return fakeComments, nil
		},
		AddCommentToPullRequestStub: func(context.Context, provider.PullRequest, []byte) (*provider.Comment, error) {
			return &provider.Comment{
				ID: 2,
			}, nil
		},
	}

	server, err := polling.New(
		polling.WithClusterClient(fakeClient),
		polling.WithBranchPollingInterval(time.Second),
		polling.WithPollingInterval(time.Second),
		polling.WithCustomProviderURLParserFn(mockedProvider(&fakeProvider)),
		polling.WithLogger(log),
	)
	g.Expect(err).To(gomega.Succeed())

	serverDone := make(chan bool)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		server.Start(ctx)
		serverDone <- true
	}()

	prtf := &infrav1.Terraform{}
	prtfKey := client.ObjectKey{Name: "tf1-pr-1", Namespace: config.DefaultNamespace}

	t.Log("A new branch planner Terraform resource should be created.")
	g.Eventually(func() error {
		return fakeClient.Get(ctx, prtfKey, prtf)
	}, eventuallyTimeout, eventuallyInterval).Should(gomega.Succeed())

	g.Expect(fakeProvider.ListPullRequestsCallCount()).To(gomega.BeNumerically(">", 0))

	t.Log("When the Pull Request has a new comment with !replan, a replan request should be sent")
	fakeComments = append(fakeComments, &provider.Comment{
		ID:   123,
		Link: "",
		Body: "!replan",
	})
	oldReconcileRequestAnnotation := ""
	if prtf.Annotations != nil {
		oldReconcileRequestAnnotation = prtf.Annotations[meta.ReconcileRequestAnnotation]
	}
	g.Eventually(func() error {
		if err := fakeClient.Get(ctx, prtfKey, prtf); err != nil {
			return nil
		}
		if prtf.Annotations == nil {
			return fmt.Errorf("resource has no annotations")
		}
		if prtf.Annotations[config.AnnotationCommentIDKey] != "2" {
			return fmt.Errorf("%q is not up to date: %s", config.AnnotationCommentIDKey, prtf.Annotations[config.AnnotationCommentIDKey])
		}
		if prtf.Annotations[meta.ReconcileRequestAnnotation] == oldReconcileRequestAnnotation {
			return fmt.Errorf("annotation %q is not up to date: %s", meta.ReconcileRequestAnnotation, prtf.Annotations[meta.ReconcileRequestAnnotation])
		}

		return nil
	}, eventuallyTimeout, eventuallyInterval).Should(gomega.Succeed())

	t.Log("As we close the Pull Request, the branch planner Terraform resource should be deleted.")
	fakePRConf.closed = true

	g.Eventually(func() error {
		return fakeClient.Get(ctx, prtfKey, prtf)
	}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.Succeed())

	cancel()
	<-serverDone
}

func testResources(namespace string) []client.Object {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "branch-planner-config",
			Namespace: namespace,
		},
		Data: map[string]string{},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "branch-planner-token",
			Namespace: namespace,
		},
	}
	source := &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tf1",
			Namespace: namespace,
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/weaveworks/tf-conrtoller",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "main",
			},
		},
	}
	terraform := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tf1",
			Namespace: namespace,
		},
		Spec: infrav1.TerraformSpec{
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      sourcev1.GitRepositoryKind,
				Name:      source.GetName(),
				Namespace: source.GetNamespace(),
			},
		},
	}
	return []client.Object{configMap, secret, source, terraform}
}
