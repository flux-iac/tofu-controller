package polling

import (
	"context"
	"testing"

	"github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/internal/git/provider"
)

// This checks poll can be called with a little setting-up, with no
// result expected.
func Test_poll_empty(t *testing.T) {
	g := gomega.NewWithT(t)
	ns := newNamespace(g)

	// Create a source for the Terraform object to point to
	source := &sourcev1.GitRepository{
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/weaveworks/tf-controller",
		},
	}
	source.SetName("original-source")
	source.SetNamespace(ns.GetName())
	expectToSucceed(g, k8sClient.Create(context.TODO(), source))

	// Create a Terraform object to be the template
	original := &infrav1.Terraform{
		Spec: infrav1.TerraformSpec{
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Name: source.GetName(),
				Kind: "GitRepository",
			},
		},
	}
	original.SetNamespace(ns.GetName())
	original.SetName("original")
	expectToSucceed(g, k8sClient.Create(context.TODO(), original))

	// This fakes a provider for the server to use.
	var prs []provider.PullRequest

	// Only WithClusterClient is really needed; the unexported option
	// lets us supply the fake provider.
	server, err := New(
		WithClusterClient(k8sClient),
	)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Now we'll run `poll` to step the server once, and afterwards,
	// we should be able to see what it did.
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	server.reconcile(ctx, original, source, prs)

	// We expect it to have done nothing! So, check it didn't create
	// any more Terraform or source objects.
	var list infrav1.TerraformList
	expectToSucceed(g, k8sClient.List(context.TODO(), &list, &client.ListOptions{
		Namespace: ns.GetName(),
	}))
	expectToEqual(g, len(list.Items), 1) // just the original
	expectToEqual(g, list.Items[0].GetName(), original.GetName())

	var srclist sourcev1.GitRepositoryList
	expectToSucceed(g, k8sClient.List(context.TODO(), &srclist, &client.ListOptions{
		Namespace: ns.GetName(),
	}))
	expectToEqual(g, len(list.Items), 1) // just `source`

	t.Cleanup(func() { expectToSucceed(g, k8sClient.Delete(context.TODO(), ns)) })
}
