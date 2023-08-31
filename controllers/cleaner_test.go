package controllers

import (
	"context"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func waitResourceToBeDelete(g gomega.Gomega, resource client.Object) {
	ctx := context.Background()
	key := types.NamespacedName{Namespace: resource.GetNamespace(), Name: resource.GetName()}

	g.Expect(k8sClient.Delete(ctx, resource)).Should(gomega.Succeed())
	g.Eventually(func() error {
		return k8sClient.Get(ctx, key, resource)
	}, timeout, interval).ShouldNot(gomega.Succeed())
}
