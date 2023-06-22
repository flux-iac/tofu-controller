package polling

import (
	"context"
	"fmt"

	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/internal/informer/bbp"
	corev1 "k8s.io/api/core/v1"
	k8sFields "k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *Server) getTerraformObject(ctx context.Context, ref client.ObjectKey) (*v1alpha2.Terraform, error) {
	obj := &v1alpha2.Terraform{}
	err := s.clusterClient.Get(ctx, ref, obj)
	if err != nil {
		return nil, fmt.Errorf("unable to get Terraform: %w", err)
	}

	return obj, nil
}

func (s *Server) listTerraformObjects(ctx context.Context, namespace string, fields map[string]string) (*v1alpha2.TerraformList, error) {
	objList := &v1alpha2.TerraformList{}

	if err := s.clusterClient.List(ctx, objList,
		client.MatchingFieldsSelector{
			Selector: k8sFields.SelectorFromSet(fields),
		},
		client.InNamespace(namespace),
	); err != nil {
		return nil, fmt.Errorf("unable to list Terraform objects: %w", err)
	}

	return objList, nil
}

func (s *Server) getSource(ctx context.Context, tf *v1alpha2.Terraform) (*sourcev1b2.GitRepository, error) {
	if tf.Spec.SourceRef.Kind != sourcev1b2.GitRepositoryKind {
		return nil, fmt.Errorf("branch based planner does not support source kind: %s", tf.Spec.SourceRef.Kind)
	}

	ref := client.ObjectKey{
		Namespace: tf.Spec.SourceRef.Namespace,
		Name:      tf.Spec.SourceRef.Name,
	}
	obj := &sourcev1b2.GitRepository{}
	err := s.clusterClient.Get(ctx, ref, obj)
	if err != nil {
		return nil, fmt.Errorf("unable to get Source: %w", err)
	}

	return obj, nil
}

func (s *Server) listSources(ctx context.Context, tf *v1alpha2.Terraform, fields map[string]string) (*sourcev1b2.GitRepositoryList, error) {
	if tf.Spec.SourceRef.Kind != sourcev1b2.GitRepositoryKind {
		return nil, fmt.Errorf("branch based planner does not support source kind: %s", tf.Spec.SourceRef.Kind)
	}

	objList := &sourcev1b2.GitRepositoryList{}
	err := s.clusterClient.List(ctx, objList, client.MatchingFieldsSelector{
		Selector: k8sFields.SelectorFromSet(fields),
	},
		client.InNamespace(tf.Spec.SourceRef.Namespace))
	if err != nil {
		return nil, fmt.Errorf("unable to list Sources: %w", err)
	}

	return objList, nil
}

func (s *Server) createTerraform(ctx context.Context, originalTF *v1alpha2.Terraform, originalSource *sourcev1b2.GitRepository, branch string, prID string) error {
	tf := originalTF.DeepCopy()

	tf.SetName(s.createObjectName(tf.GetName(), branch, prID))

	tf.SetAnnotations(s.createAnnotations(tf.GetAnnotations(), branch, prID))

	msg := fmt.Sprintf("Terraform object %s in the namespace %s", tf.ObjectMeta.Name, tf.ObjectMeta.Namespace)

	source, err := s.createSource(ctx, originalSource, branch, prID)
	if err != nil {
		return fmt.Errorf("unable to create Source for %s: %w", msg, err)
	}

	tf.Spec.SourceRef.Name = source.GetName()

	tf.Spec.PlanOnly = true
	tf.Spec.StoreReadablePlan = "human"

	tf.Spec.WriteOutputsToSecret.Name = s.createObjectName(tf.Spec.WriteOutputsToSecret.Name, branch, prID)

	if err = s.clusterClient.Create(ctx, tf); err != nil {
		return fmt.Errorf("unable to create %s: %w", msg, err)
	}

	s.log.Info("created %s", msg)

	return nil
}

func (s *Server) createSource(ctx context.Context, originalSource *sourcev1b2.GitRepository, branch string, prID string) (*sourcev1b2.GitRepository, error) {
	source := originalSource.DeepCopy()

	source.SetName(s.createObjectName(source.GetName(), branch, prID))

	source.Spec.Reference.Branch = branch

	source.SetAnnotations(s.createAnnotations(source.GetAnnotations(), branch, prID))

	msg := fmt.Sprintf("Source %s in the namespace %s", source.ObjectMeta.Name, source.ObjectMeta.Namespace)

	if err := s.clusterClient.Create(ctx, source); err != nil {
		return nil, fmt.Errorf("unable to create %s: %w", msg, err)
	}

	s.log.Info("created %s", msg)

	return source, nil
}

func (s *Server) createObjectName(name string, branch string, prID string) string {
	return fmt.Sprintf("%s-%s-%s", name, branch, prID)
}

func (s *Server) createAnnotations(annotations map[string]string, branch string, prID string) map[string]string {
	if annotations == nil {
		return map[string]string{
			bbp.AnnotationBBPKey:  bbp.AnnotationBBPValue,
			bbp.AnnotationPRIDKey: prID,
		}
	} else {
		annotations[bbp.AnnotationBBPKey] = bbp.AnnotationBBPValue
		annotations[bbp.AnnotationPRIDKey] = prID
		return annotations
	}
}

func (s *Server) deleteTerraform(ctx context.Context, tf *v1alpha2.Terraform) error {
	msg := fmt.Sprintf("Terraform %s in the namespace %s", tf.ObjectMeta.Name, tf.ObjectMeta.Namespace)

	if err := s.deleteSource(ctx, tf); err != nil {
		s.log.Info("unable to delete Source for %s: %w, err", msg)
	}

	if err := s.clusterClient.Delete(ctx, tf); err != nil {
		return fmt.Errorf("unable to delete %s: %w", msg, err)
	}

	s.log.Info("deleted %s", msg)

	return nil
}

func (s *Server) deleteSource(ctx context.Context, tf *v1alpha2.Terraform) error {
	source, err := s.getSource(ctx, tf)
	if err != nil {
		return fmt.Errorf("Error getting Source for Terraform %s in the namespace %s: %w", tf.ObjectMeta.Name, tf.ObjectMeta.Namespace, err)
	}

	msg := fmt.Sprintf("Source %s in the namespace %s", source.ObjectMeta.Name, source.ObjectMeta.Namespace)

	if err := s.clusterClient.Delete(ctx, source); err != nil {
		return fmt.Errorf("unable to delete %s: %w", msg, err)
	}

	s.log.Info("deleted %s", msg)

	return nil
}

func (s *Server) getSecret(ctx context.Context, ref client.ObjectKey) (*corev1.Secret, error) {
	obj := &corev1.Secret{}
	err := s.clusterClient.Get(ctx, ref, obj)
	if err != nil {
		return nil, fmt.Errorf("unable to get Secret: %w", err)
	}

	return obj, nil
}
