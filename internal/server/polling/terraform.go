package polling

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/fluxcd/pkg/runtime/acl"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sLabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/flux-iac/tofu-controller/internal/config"
	bpconfig "github.com/flux-iac/tofu-controller/internal/config"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

func (s *Server) getTerraformObject(ctx context.Context, ref client.ObjectKey) (*infrav1.Terraform, error) {
	obj := &infrav1.Terraform{}
	if err := s.clusterClient.Get(ctx, ref, obj); err != nil {
		return nil, fmt.Errorf("unable to get Terraform: %w", err)
	}

	return obj, nil
}

func (s *Server) listTerraformObjects(ctx context.Context, namespace string, labels map[string]string) ([]*infrav1.Terraform, error) {
	tfList := &infrav1.TerraformList{}

	opts := []client.ListOption{client.InNamespace(namespace)}

	if labels != nil {
		opts = append(opts, client.MatchingLabelsSelector{
			Selector: k8sLabels.Set(labels).AsSelector(),
		})
	}

	if err := s.clusterClient.List(ctx, tfList, opts...); err != nil {
		return nil, fmt.Errorf("unable to list Terraform objects: %w", err)
	}

	result := make([]*infrav1.Terraform, len(tfList.Items))
	for i := range tfList.Items {
		result[i] = &tfList.Items[i]
	}

	return result, nil
}

func (s *Server) getSource(ctx context.Context, tf *infrav1.Terraform) (*sourcev1.GitRepository, error) {
	if tf.Spec.SourceRef.Kind != sourcev1.GitRepositoryKind {
		return nil, fmt.Errorf("branch based planner does not support source kind: %s", tf.Spec.SourceRef.Kind)
	}

	ref := client.ObjectKey{
		Namespace: tf.GetNamespace(),
		Name:      tf.Spec.SourceRef.Name,
	}

	if ns := tf.Spec.SourceRef.Namespace; ns != "" {
		ref.Namespace = ns
	}

	if s.noCrossNamespaceRefs && ref.Namespace != tf.GetNamespace() {
		return nil, acl.AccessDeniedError(
			fmt.Sprintf("cannot access %s/%s, cross-namespace references have been disabled", tf.Spec.SourceRef.Kind, ref),
		)
	}

	obj := &sourcev1.GitRepository{}
	if err := s.clusterClient.Get(ctx, ref, obj); err != nil {
		return nil, fmt.Errorf("unable to get Source: %w", err)
	}

	return obj, nil
}

func (s *Server) reconcileTerraform(ctx context.Context, originalTF *infrav1.Terraform, originalSource *sourcev1.GitRepository, branch string, prID string, interval time.Duration) error {
	tfName := config.PullRequestObjectName(originalTF.Name, prID)
	msg := fmt.Sprintf("Terraform object %s in the namespace %s", tfName, originalTF.Namespace)
	source, err := s.reconcileSource(ctx, originalTF.Name, originalSource, branch, prID, interval)
	if err != nil {
		return fmt.Errorf("unable to reconcile Source for %s: %w", msg, err)
	}

	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tfName,
			Namespace: originalTF.Namespace,
		},
	}

	branchLabels := s.createLabels(originalTF.Labels, originalTF.Name, branch, prID)
	op, err := controllerutil.CreateOrUpdate(ctx, s.clusterClient, tf, func() error {
		spec := originalTF.Spec.DeepCopy()

		spec.SourceRef.Name = source.Name
		spec.SourceRef.Namespace = source.Namespace

		// DestroyResourcesOnDeletion must be false, otherwise plan deletion will destroy resources
		spec.DestroyResourcesOnDeletion = false
		spec.PlanOnly = true
		spec.StoreReadablePlan = "human"

		// We don't need to examine or use the outputs of the plan
		spec.WriteOutputsToSecret = nil

		spec.ApprovePlan = ""
		spec.Force = false

		// Support branch planning for Terraform Cloud
		// By using local state and a local backend for the branch plan object
		if spec.Cloud != nil || spec.CliConfigSecretRef != nil {
			spec.Cloud = nil
			spec.CliConfigSecretRef = nil
		}

		if spec.BackendConfig == nil {
			spec.BackendConfig = &infrav1.BackendConfigSpec{
				SecretSuffix:    originalTF.Name,
				InClusterConfig: true,
			}
		}

		tf.Spec = *spec

		tf.SetLabels(branchLabels)

		return nil
	})
	if err != nil {
		return fmt.Errorf("reconcile failed for %s: %w", msg, err)
	} else if op != controllerutil.OperationResultNone {
		s.log.Info(fmt.Sprintf("%s successfully reconciled", msg), "operation", op)
	}

	return nil
}

func (s *Server) reconcileSource(ctx context.Context, tfName string, originalSource *sourcev1.GitRepository, branch string, prID string, interval time.Duration) (*sourcev1.GitRepository, error) {
	sourceName := config.SourceName(tfName, originalSource.Name, prID)
	msg := fmt.Sprintf("Source %s in the namespace %s", sourceName, originalSource.Namespace)
	source := &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceName,
			Namespace: originalSource.Namespace,
		},
		Spec: originalSource.Spec,
	}
	branchLabels := s.createLabels(originalSource.Labels, originalSource.Name, branch, prID)

	op, err := controllerutil.CreateOrUpdate(ctx, s.clusterClient, source, func() error {
		source.SetLabels(branchLabels)

		spec := originalSource.Spec.DeepCopy()

		if spec.Reference != nil {
			spec.Reference.Branch = branch
		} else {
			spec.Reference = &sourcev1.GitRepositoryRef{Branch: branch}
		}
		spec.Interval = metav1.Duration{
			Duration: interval,
		}

		source.Spec = *spec

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reconcile failed for %s: %w", msg, err)
	} else if op != controllerutil.OperationResultNone {
		s.log.Info(fmt.Sprintf("%s successfully reconciled", msg), "operation", op)
	}

	return source, nil
}

func (s *Server) createLabels(labels map[string]string, originalName string, branch string, prID string) map[string]string {
	resultLabels := make(map[string]string)
	maps.Copy(resultLabels, labels)

	resultLabels[bpconfig.LabelKey] = bpconfig.LabelValue
	resultLabels[bpconfig.LabelPrimaryResourceKey] = originalName
	resultLabels[bpconfig.LabelPRIDKey] = prID
	return resultLabels
}

func (s *Server) deleteTerraformAndSource(ctx context.Context, tf *infrav1.Terraform) error {
	const (
		pollInterval = 5 * time.Second // Interval to check the deletion status
		pollTimeout  = 2 * time.Minute // Total time before timing out
	)

	tfMsg := fmt.Sprintf("Terraform %s in the namespace %s", tf.Name, tf.Namespace)

	// Get source, but not yet delete it
	source, err := s.getSource(ctx, tf)
	if err != nil {
		return fmt.Errorf("unable to get Source for %s: %w", tfMsg, err)
	}

	// Delete Terraform object
	if err := s.clusterClient.Delete(ctx, tf); err != nil {
		return fmt.Errorf("unable to delete %s: %w", tfMsg, err)
	}

	// We have to wait for the Terraform object to be deleted before deleting the source
	err = wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		if err := s.clusterClient.Get(ctx, client.ObjectKey{Name: tf.Name, Namespace: tf.Namespace}, tf); err != nil {
			if errors.IsNotFound(err) {
				return true, nil // Terraform object is deleted
			}
			return false, err // An error occurred
		}
		return false, nil // Terraform object still exists
	})

	if err != nil {
		return fmt.Errorf("error waiting for %s to be deleted: %w", tfMsg, err)
	}

	s.log.Info(fmt.Sprintf("deleted %s", tfMsg))

	sourceMsg := fmt.Sprintf("Source %s in the namespace %s", source.Name, source.Namespace)
	if err := s.clusterClient.Delete(ctx, source); err != nil {
		return fmt.Errorf("unable to delete %s: %w", sourceMsg, err)
	}

	s.log.Info(fmt.Sprintf("deleted %s", sourceMsg))

	return nil
}

func (s *Server) getSecret(ctx context.Context, ref client.ObjectKey) (*corev1.Secret, error) {
	obj := &corev1.Secret{}
	if err := s.clusterClient.Get(ctx, ref, obj); err != nil {
		return nil, fmt.Errorf("unable to get Secret: %w", err)
	}

	return obj, nil
}
