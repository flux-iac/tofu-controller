package controllers

import (
	"context"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Note: infrav1 must be imported explicitly in THIS file even though other
// files in the same `controllers` package already import it — Go import
// visibility is per-file, not per-package.

// Test_000363_AdditionalSourceNamespaces proves the exact mechanism this PR
// relies on: a cache scoped to one namespace via DefaultNamespaces, with
// ByObject widening ONLY the GitRepository kind to an additional namespace,
// correctly (a) allows reading GitRepository objects in that additional
// namespace, (b) still refuses to read Terraform-kind objects there
// (isolation preserved — this is the property that ruled out the
// alternative "just widen DefaultNamespaces" design), and (c) still refuses
// to read GitRepository objects in a third, unlisted namespace (no
// accidental over-broadening).
func Test_000363_AdditionalSourceNamespaces(t *testing.T) {
	g := NewWithT(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		shardNS = "test-shard-a"
		srcNS   = "test-flux-system"
		otherNS = "test-other-ns"
	)

	for _, ns := range []string{shardNS, srcNS, otherNS} {
		g.Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		})).To(Succeed())
	}

	srcInAdditionalNS := &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{Name: "src-in-additional-ns", Namespace: srcNS},
		Spec: sourcev1.GitRepositorySpec{
			URL:      "https://github.com/flux-iac/tofu-controller",
			Interval: metav1.Duration{Duration: time.Hour},
		},
	}
	g.Expect(k8sClient.Create(ctx, srcInAdditionalNS)).To(Succeed())

	tfInAdditionalNS := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{Name: "tf-in-additional-ns", Namespace: srcNS},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: infrav1.ApprovePlanAutoValue,
			Interval:    metav1.Duration{Duration: time.Hour},
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind: "GitRepository",
				Name: "src-in-additional-ns",
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, tfInAdditionalNS)).To(Succeed())

	srcInOtherNS := &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{Name: "src-in-other-ns", Namespace: otherNS},
		Spec: sourcev1.GitRepositorySpec{
			URL:      "https://github.com/flux-iac/tofu-controller",
			Interval: metav1.Duration{Duration: time.Hour},
		},
	}
	g.Expect(k8sClient.Create(ctx, srcInOtherNS)).To(Succeed())

	// This cache is built with its own scheme, deliberately separate from
	// TestMain's manager/reconciler setup — TestMain's scheme is a
	// function-local variable there, not shared package state, and this
	// test is designed to be fully self-contained rather than reach into
	// TestMain's internals. It registers the same types TestMain's scheme
	// does (client-go's scheme, source-controller's API, and this
	// controller's API).
	scheme := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	g.Expect(sourcev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(infrav1.AddToScheme(scheme)).To(Succeed())

	scopedCache, err := cache.New(cfg, cache.Options{
		Scheme: scheme,
		DefaultNamespaces: map[string]cache.Config{
			shardNS: {},
		},
		ByObject: map[client.Object]cache.ByObject{
			&sourcev1.GitRepository{}: {
				Namespaces: map[string]cache.Config{shardNS: {}, srcNS: {}},
			},
		},
	})
	g.Expect(err).NotTo(HaveOccurred())

	cacheCtx, cacheCancel := context.WithCancel(ctx)
	defer cacheCancel()
	go func() {
		_ = scopedCache.Start(cacheCtx)
	}()
	g.Expect(scopedCache.WaitForCacheSync(cacheCtx)).To(BeTrue())

	// (a) a GitRepository in the additional namespace IS readable — the fix working.
	var gotSrc sourcev1.GitRepository
	g.Expect(scopedCache.Get(ctx, types.NamespacedName{Namespace: srcNS, Name: "src-in-additional-ns"}, &gotSrc)).To(Succeed())

	// (b) a Terraform CR in that SAME additional namespace is NOT readable —
	// reconcile-scope isolation preserved, even though its namespace is
	// readable for GitRepository purposes.
	var gotTF infrav1.Terraform
	err = scopedCache.Get(ctx, types.NamespacedName{Namespace: srcNS, Name: "tf-in-additional-ns"}, &gotTF)
	g.Expect(err).To(HaveOccurred())

	// (c) a GitRepository in a third, UNLISTED namespace is still NOT
	// readable — no accidental over-broadening beyond what was configured.
	var gotOtherSrc sourcev1.GitRepository
	err = scopedCache.Get(ctx, types.NamespacedName{Namespace: otherNS, Name: "src-in-other-ns"}, &gotOtherSrc)
	g.Expect(err).To(HaveOccurred())
}
