package controllers

import (
	"context"
	"fmt"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.cgithub.com/flux-iac/tofu-controller
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	if _, ok := resource.(*infrav1.Terraform); ok {
		waitDefaultSecretsToBeDeletedForTerraform(g, resource)
	}
}

func waitDefaultSecretsToBeDeletedForTerraform(g gomega.Gomega, resource client.Object) {
	planSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("tfplan-default-%s", resource.GetName()),
			Namespace: "flux-system",
		},
	}
	stateSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("tfstate-default-%s", resource.GetName()),
			Namespace: "flux-system",
		},
	}

	waitResourceToBeDelete(g, planSecret)
	waitResourceToBeDelete(g, stateSecret)
}
