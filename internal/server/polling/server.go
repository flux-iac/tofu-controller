package polling

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/weaveworks/tf-controller/internal/git/provider"
	"github.com/weaveworks/tf-controller/internal/informer/bbp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const DefaultPollingInterval = time.Second * 30

type Server struct {
	log             logr.Logger
	clusterClient   client.Client
	configMapRef    client.ObjectKey
	pollingInterval time.Duration
}

func New(options ...Option) (*Server, error) {
	server := &Server{log: logr.Discard()}

	for _, opt := range options {
		if err := opt(server); err != nil {
			return nil, err
		}
	}

	return server, nil
}

func (s *Server) Start(ctx context.Context) error {
	tick := time.Tick(s.pollingInterval)
	for {
		select {
		case <-ctx.Done():
			return nil

		case <-tick:
			// Read the config in each iteration. The idea behind this decision is to
			// allow the user to change the list of resources without the need of
			// restart of the pod.
			// It can be a bit smarter like using a time.Ticker and refresh config
			// periodically.
			config, err := s.readConfig(ctx)
			if err != nil {
				return err
			}

			secret, err := s.getSecret(ctx, client.ObjectKey{
				Namespace: config.SecretNamespace,
				Name:      config.SecretName,
			})
			if err != nil {
				s.log.Error(err, "failed to get secret")
			}

			for _, resource := range config.Resources {
				if err := s.poll(ctx, resource, secret); err != nil {
					s.log.Error(err, "failed to check pull request")
				}
			}
		}
	}
}

func (s *Server) poll(ctx context.Context, resource types.NamespacedName, secret *corev1.Secret) error {
	if secret == nil {
		return fmt.Errorf("secret is not defined")
	}

	tf, err := s.getTerraformObject(ctx, resource)
	if err != nil {
		return fmt.Errorf("failed to get Terraform object: %w", err)
	}

	source, err := s.getSource(ctx, tf)
	if err != nil {
		return fmt.Errorf("failed to get Source object: %w", err)
	}

	gitProvider, repo, err := provider.FromURL(
		source.Spec.URL,
		provider.WithLogger(s.log),
		provider.WithToken("api-token", string(secret.Data["token"])),
	)
	if err != nil {
		return fmt.Errorf("failed to get git provider: %w", err)
	}

	prs, err := gitProvider.ListPullRequests(ctx, repo)
	if err != nil {
		return fmt.Errorf("failed to list pull requests: %w", err)
	}

	fields := map[string]string{
		fmt.Sprintf("%s.%s", bbp.AnnotationsKey, bbp.AnnotationBBPKey): bbp.AnnotationBBPValue,
	}

	// List Terraform objects, created by BBP.
	_, err = s.listTerraformObjects(ctx, resource.Namespace, fields)
	if err != nil {
		return fmt.Errorf("failed to list Terraform objects: %w", err)
	}

	// List Sources, created by BBP.
	_, err = s.listSources(ctx, tf, fields)
	if err != nil {
		return fmt.Errorf("failed to list Sources: %w", err)
	}

	// TODO: Create list of objects to delete.
	// TODO: Create list of objects to create.

	// Process the PRs.
	for _, pr := range prs {
		s.log.Info("pull request", "pr", pr)
	}

	return nil
}
