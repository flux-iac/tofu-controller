package controllers

import (
	"context"

	"github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const cleanupTimeoutSeconds = 60

func waitResourceToBeDelete(g gomega.Gomega, resource client.Object) {
	ctx := context.Background()
	key := types.NamespacedName{Namespace: resource.GetNamespace(), Name: resource.GetName()}

	err := k8sClient.Get(ctx, key, resource)
	if apierrors.IsNotFound(err) {
		return
	}

	g.Expect(k8sClient.Delete(ctx, resource)).Should(gomega.Succeed())
	g.Eventually(func() error {
		return k8sClient.Get(ctx, key, resource)
	}, cleanupTimeoutSeconds, interval).ShouldNot(gomega.Succeed())
}
