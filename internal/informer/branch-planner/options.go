package branchplanner

import (
	"github.com/go-logr/logr"
	"github.com/weaveworks/tf-controller/internal/git/provider"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func WithLogger(log logr.Logger) Option {
	return func(i *Informer) error {
		i.log = log

		return nil
	}
}

func WithClusterClient(clusterClient client.Client) Option {
	return func(i *Informer) error {
		i.client = clusterClient

		return nil
	}
}

func WithGitProvider(provider provider.Provider) Option {
	return func(i *Informer) error {
		i.gitProvider = provider

		return nil

	}
}

func WithSharedInformer(informer cache.SharedIndexInformer) Option {
	return func(i *Informer) error {
		i.sharedInformer = informer

		return nil
	}
}
