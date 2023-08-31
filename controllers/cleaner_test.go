package controllers

import (
	"sync"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type resourceToBeDeleted struct {
	Namespace string
	Name      string
	Object    client.Object
}

func waitResourceToBeDelete(g gomega.Gomega, resource resourceToBeDeleted) {
	g.Eventually(func() error {
		key := types.NamespacedName{Namespace: resource.Namespace, Name: resource.Name}

		return k8sClient.Get(ctx, key, resource.Object)
	}, timeout, interval).ShouldNot(gomega.Succeed())
}

func waitResourcesToBeDelete(g gomega.Gomega, resources []resourceToBeDeleted) {
	var wg sync.WaitGroup

	for _, resource := range resources {
		wg.Add(1)
		go func(resource resourceToBeDeleted) {
			waitResourceToBeDelete(g, resource)
			wg.Done()
		}(resource)
	}

	wg.Wait()
}
