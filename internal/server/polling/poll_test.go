package polling

import (
	"context"
	"fmt"
	"testing"

	bpconfig "github.com/weaveworks/tf-controller/internal/config"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/weaveworks/tf-controller/internal/git/provider/providerfakes"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/internal/git/provider"
)

// This checks poll can be called with a little setting-up, with no
// result expected.
func Test_poll_empty(t *testing.T) {
	g := gomega.NewWithT(t)
	ns := newNamespace(t, g)

	// Create a source for the Terraform object to point to
	source := &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "original-source",
			Namespace: ns.Name,
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/weaveworks/tf-controller",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "main",
			},
		},
	}
	expectToSucceed(t, g, k8sClient.Create(context.TODO(), source))

	// Create a Terraform object to be the template.
	original := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "original",
			Namespace: ns.Name,
		},
		Spec: infrav1.TerraformSpec{
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Name: source.Name,
				Kind: "GitRepository",
			},
		},
	}
	expectToSucceed(t, g, k8sClient.Create(context.TODO(), original))

	// This fakes a provider for the server to use.
	var prs []provider.PullRequest

	server, err := New(
		WithClusterClient(k8sClient),
	)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Now we'll run `reconcile` to step the server once, and afterwards,
	// we should be able to see what it did.
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	expectToSucceed(t, g, server.reconcile(ctx, original, source, prs, &providerfakes.FakeProvider{}))

	// We expect it to have done nothing! So, check it didn't create
	// any more Terraform or source objects.
	var tfList infrav1.TerraformList
	expectToSucceed(t, g, k8sClient.List(context.TODO(), &tfList, &client.ListOptions{
		Namespace: ns.Name,
	}))
	expectToEqual(t, g, len(tfList.Items), 1) // just the original
	expectToEqual(t, g, tfList.Items[0].Name, original.Name)

	var srcList sourcev1.GitRepositoryList
	expectToSucceed(t, g, k8sClient.List(context.TODO(), &srcList, &client.ListOptions{
		Namespace: ns.Name,
	}))
	expectToEqual(t, g, len(srcList.Items), 1) // just `source`
	expectToEqual(t, g, srcList.Items[0].Name, source.Name)

	t.Cleanup(func() { expectToSucceed(t, g, k8sClient.Delete(context.TODO(), ns)) })
}

// This checks that branch Terraform objects are created,
// when there are open pull requests,
// updated when the original Terraform object is updated,
// and deleted when the corresponding PRs are closed.
// The original Terraform object and source should be retained.
func Test_poll_reconcile_objects(t *testing.T) {
	g := gomega.NewWithT(t)
	ns := newNamespace(t, g)

	// Create a source for the Terraform object to point to
	source := &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "original-source",
			Namespace: ns.Name,
			Labels: map[string]string{
				"test-label": "123",
			},
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/tf-controller/helloworld",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "main",
			},
		},
	}
	expectToSucceed(t, g, k8sClient.Create(context.TODO(), source))

	// Create a Terraform object to be the template.
	original := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "original",
			Namespace: ns.Name,
			Labels: map[string]string{
				"test-label": "abc",
			},
		},
		Spec: infrav1.TerraformSpec{
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Name: source.Name,
				Kind: "GitRepository",
			},
			WriteOutputsToSecret: &infrav1.WriteOutputsToSecretSpec{
				Name: "test-secret",
			},
			ApprovePlan: "should be cleared",
			Force:       true, // should be set false on clone.
		},
	}
	expectToSucceed(t, g, k8sClient.Create(context.TODO(), original))

	// This fakes a provider for the server to use.
	repo := provider.Repository{
		Project: "fake-project",
		Org:     "fake-org",
		Name:    "fake-name",
	}
	prs := []provider.PullRequest{
		{
			Repository: repo,
			Number:     1,
			BaseBranch: "main",
			HeadBranch: "test-branch-1",
		},
		{
			Repository: repo,
			Number:     2,
			BaseBranch: "main",
			HeadBranch: "test-branch-2",
		},
		{
			Repository: repo,
			Number:     3,
			BaseBranch: "main",
			HeadBranch: "test-branch-3",
		},
	}

	server, err := New(
		WithClusterClient(k8sClient),
	)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Now we'll run `reconcile` to step the server once, and afterwards,
	// we should be able to see what it did.
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	expectToSucceed(t, g, server.reconcile(ctx, original, source, prs, &providerfakes.FakeProvider{}))

	// We expect the branch TF objects and corresponding sources
	// to be created for each PR
	// and the original object and source to be retained.

	// Check that the Terraform objects are created with expected fields.
	var tfList infrav1.TerraformList
	expectToSucceed(t, g, k8sClient.List(context.TODO(), &tfList, &client.ListOptions{
		Namespace: ns.Name,
	}))

	expectToEqual(t, g, len(tfList.Items), 4)
	// The first one is the original Terraform object.
	expectToEqual(t, g, tfList.Items[0].Name, original.Name)

	// Ignore the first one as it's the original resource.
	for idx, item := range tfList.Items[1:] {
		expectToEqual(t, g, item.Name, fmt.Sprintf("%s-pr-%d", original.Name, idx+1))
		expectToEqual(t, g, item.Spec.SourceRef.Name, fmt.Sprintf("%s-source-pr-%d", original.Name, idx+1))
		expectToEqual(t, g, item.Spec.SourceRef.Namespace, ns.Name)
		expectToEqual(t, g, item.Spec.PlanOnly, true)
		expectToEqual(t, g, item.Spec.StoreReadablePlan, "human")
		expectToEqual(t, g, item.Spec.ApprovePlan, "")
		expectToEqual(t, g, item.Spec.Force, false)
		g.Expect(item.Spec.WriteOutputsToSecret).To(gomega.BeNil()) // we don't need to use the output Secret of the plan
		expectToEqual(t, g, item.Labels[bpconfig.LabelKey], bpconfig.LabelValue)
		expectToEqual(t, g, item.Labels["test-label"], "abc")
		expectToEqual(t, g, item.Labels[bpconfig.LabelPRIDKey], fmt.Sprint(idx+1))
	}

	// Check that the Source objects are created with all expected fields.
	var srcList sourcev1.GitRepositoryList
	expectToSucceed(t, g, k8sClient.List(context.TODO(), &srcList, &client.ListOptions{
		Namespace: ns.Name,
	}))

	expectToEqual(t, g, len(srcList.Items), 4)
	// The first one is the original Source object.
	expectToEqual(t, g, srcList.Items[0].Name, source.Name)

	// Ignore the first one as it's the original resource.
	for idx, item := range srcList.Items[1:] {
		expectToEqual(t, g, item.Name, fmt.Sprintf("%s-pr-%d", source.Name, idx+1))
		expectToEqual(t, g, item.Spec.Reference.Branch, fmt.Sprintf("test-branch-%d", idx+1))
		expectToEqual(t, g, item.Labels[bpconfig.LabelKey], bpconfig.LabelValue)
		expectToEqual(t, g, item.Labels["test-label"], "123")
		expectToEqual(t, g, item.Labels[bpconfig.LabelPRIDKey], fmt.Sprint(idx+1))
	}

	// Check that branch Terraform objects are updated
	// after the original Terraform object is updated.
	secretName := "new-test-secret"
	original.Labels["test-label"] = "xyz"
	original.Spec.WriteOutputsToSecret.Name = secretName

	expectToSucceed(t, g, k8sClient.Update(context.TODO(), original))
	expectToSucceed(t, g, server.reconcile(ctx, original, source, prs, &providerfakes.FakeProvider{}))

	tfList.Items = nil

	expectToSucceed(t, g, k8sClient.List(context.TODO(), &tfList, &client.ListOptions{
		Namespace:     ns.Name,
		LabelSelector: labels.Set{bpconfig.LabelKey: bpconfig.LabelValue}.AsSelector(),
	}))

	for _, item := range tfList.Items {
		expectToEqual(t, g, item.Labels["test-label"], "xyz")
		g.Expect(item.Spec.WriteOutputsToSecret).To(gomega.BeNil())
	}

	// Check that corresponding Terraform objects and Sources are deleted
	// after PRs are deleted
	// and the original Terraform object and source are retained.
	prs = prs[2:]

	expectToSucceed(t, g, server.reconcile(ctx, original, source, prs, &providerfakes.FakeProvider{}))

	tfList.Items = nil

	expectToSucceed(t, g, k8sClient.List(context.TODO(), &tfList, &client.ListOptions{
		Namespace: ns.Name,
	}))

	expectToEqual(t, g, len(tfList.Items), 2)
	expectToEqual(t, g, tfList.Items[0].Name, original.Name)
	expectToEqual(t, g, tfList.Items[1].Name, original.Name+"-pr-3")

	srcList.Items = nil

	expectToSucceed(t, g, k8sClient.List(context.TODO(), &srcList, &client.ListOptions{
		Namespace: ns.Name,
	}))

	expectToEqual(t, g, len(srcList.Items), 2)
	expectToEqual(t, g, srcList.Items[0].Name, source.Name)
	expectToEqual(t, g, srcList.Items[1].Name, source.Name+"-pr-3")

	t.Cleanup(func() { expectToSucceed(t, g, k8sClient.Delete(context.TODO(), ns)) })
}

// If there are no Terraform changes in a Pull Request, and
// `.spec.BranchPlanner.EnablePathScope` is true, we expect no new resources are
// created for that Pull Request.
func Test_poll_noPathChanges(t *testing.T) {
	g := gomega.NewWithT(t)
	ns := newNamespace(t, g)

	// Create a source for the Terraform object to point to
	source := &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "original-source",
			Namespace: ns.Name,
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/weaveworks/tf-controller",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "main",
			},
		},
	}
	expectToSucceed(t, g, k8sClient.Create(context.TODO(), source))

	// Create a Terraform object to be the template.
	original := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "original",
			Namespace: ns.Name,
		},
		Spec: infrav1.TerraformSpec{
			Path: "./infra/",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Name: source.Name,
				Kind: "GitRepository",
			},
			BranchPlanner: &infrav1.BranchPlanner{
				EnablePathScope: true,
			},
		},
	}
	expectToSucceed(t, g, k8sClient.Create(context.TODO(), original))

	repo := provider.Repository{
		Project: "fake-project",
		Org:     "fake-org",
		Name:    "fake-name",
	}
	prs := []provider.PullRequest{
		{
			Repository: repo,
			Number:     1,
			BaseBranch: "main",
			HeadBranch: "test-branch-1",
		},
	}
	prChanges := []provider.Change{
		{
			Path:      "cmd/project/main.go",
			Additions: 2,
			Added:     true,
		},
	}

	gitProvider := &providerfakes.FakeProvider{}
	gitProvider.ListPullRequestChangesReturns(prChanges, nil)

	server, err := New(
		WithClusterClient(k8sClient),
	)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Now we'll run `reconcile` to step the server once, and afterwards,
	// we should be able to see what it did.
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	expectToSucceed(t, g, server.reconcile(ctx, original, source, prs, gitProvider))

	// We expect it to have done nothing! So, check it didn't create
	// any more Terraform or source objects.
	var tfList infrav1.TerraformList
	expectToSucceed(t, g, k8sClient.List(context.TODO(), &tfList, &client.ListOptions{
		Namespace: ns.Name,
	}))
	expectToEqual(t, g, len(tfList.Items), 1, "terraform list") // just the original
	expectToEqual(t, g, tfList.Items[0].Name, original.Name)

	var srcList sourcev1.GitRepositoryList
	expectToSucceed(t, g, k8sClient.List(context.TODO(), &srcList, &client.ListOptions{
		Namespace: ns.Name,
	}))
	expectToEqual(t, g, len(srcList.Items), 1, "source list") // just `source`
	expectToEqual(t, g, srcList.Items[0].Name, source.Name)

	t.Cleanup(func() { expectToSucceed(t, g, k8sClient.Delete(context.TODO(), ns)) })
}
