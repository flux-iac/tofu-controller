package controllers

import (
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"testing"
	"time"
)

func TestSourceRevisionChangePredicate_Update(t *testing.T) {
	fixtureSource := &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "source",
			Namespace: "flux-system",
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/openshift-fluxv2-poc/podinfo",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "master",
			},
			Interval:          metav1.Duration{Duration: time.Second * 30},
			GitImplementation: "go-git",
		},
	}

	g := NewWithT(t)
	predicate := SourceRevisionChangePredicate{}
	var result bool

	// First false case
	result = predicate.Update(event.UpdateEvent{
		ObjectOld: nil,
		ObjectNew: nil,
	})
	g.Expect(result).To(BeFalse())

	// Second false case
	result = predicate.Update(event.UpdateEvent{
		ObjectOld: struct{ client.Object }{},
		ObjectNew: fixtureSource.DeepCopy(),
	})
	g.Expect(result).To(BeFalse())

	// Second false case
	result = predicate.Update(event.UpdateEvent{
		ObjectOld: fixtureSource.DeepCopy(),
		ObjectNew: struct{ client.Object }{},
	})
	g.Expect(result).To(BeFalse())

	// Last false case
	oldSource := fixtureSource.DeepCopy()
	oldSource.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(1),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: time.Now()},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			},
		},
		URL: server.URL() + "/file.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: time.Now()},
		},
	}

	newSource := fixtureSource.DeepCopy()
	newSource.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(1),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: time.Now()},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			},
		},
		URL: server.URL() + "/file.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: time.Now()},
		},
	}

	result = predicate.Update(event.UpdateEvent{
		ObjectOld: oldSource,
		ObjectNew: newSource,
	})
	g.Expect(result).To(BeFalse())

}
