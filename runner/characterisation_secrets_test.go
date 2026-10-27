package runner_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/flux-iac/tofu-controller/api/plan"
	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"

	. "github.com/onsi/gomega"
)

// planSecrets builds the plan secrets exactly as the runner stores them.
func planSecrets(t *testing.T, name, namespace, workspace, planID string, content []byte) []*corev1.Secret {
	t.Helper()

	p, err := plan.NewFromBytes(name, namespace, workspace, "uid-1", planID, content)
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := p.ToSecret("")
	if err != nil {
		t.Fatal(err)
	}
	return secrets
}

func outputSecret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data:       map[string][]byte{"k": []byte("v")},
	}
}

func TestCharacterisation_FinalizeSecrets(t *testing.T) {
	finalize := func(h *harness, withOutput bool) error {
		_, err := h.Client.FinalizeSecrets(t.Context(), &runner.FinalizeSecretsRequest{
			Namespace:                "default",
			Name:                     "app",
			Workspace:                "default",
			HasSpecifiedOutputSecret: withOutput,
			OutputSecretName:         "app-outputs",
		})
		return err
	}
	finalizeReply := func(h *harness) (*runner.FinalizeSecretsReply, error) {
		return h.Client.FinalizeSecrets(t.Context(), &runner.FinalizeSecretsRequest{
			Namespace: "default", Name: "app", Workspace: "default",
		})
	}
	seededPlan := func() *corev1.Secret {
		return planSecrets(t, "app", "default", "default", "plan-1", []byte("plan"))[0]
	}

	t.Run("no plan secrets", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)

		err := finalize(h, false)

		g.Expect(status.Code(err)).To(Equal(codes.NotFound))
		g.Expect(status.Convert(err).Message()).To(Equal("no existing plan secrets found to delete"))
	})

	t.Run("plan secret delete fails", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarnessWith(t, []harnessOption{withInterceptors(interceptor.Funcs{
			Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
				return errors.New("delete boom")
			},
		})}, seededPlan())

		err := finalize(h, false)

		g.Expect(status.Code(err)).To(Equal(codes.Internal))
		g.Expect(status.Convert(err).Message()).To(Equal("delete boom"))
	})

	t.Run("list fails", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarnessWith(t, []harnessOption{withInterceptors(interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return errors.New("list boom")
			},
		})}, seededPlan())

		err := finalize(h, false)

		// Internal, because the controller treats Internal as a transient
		// failure.
		g.Expect(status.Code(err)).To(Equal(codes.Internal))
		g.Expect(status.Convert(err).Message()).To(Equal("list boom"))
	})

	t.Run("output secret missing", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t, seededPlan())

		err := finalize(h, true)

		// The plan secrets were deleted, so there was something to finalize.
		g.Expect(err).NotTo(HaveOccurred())
		var left corev1.SecretList
		g.Expect(h.Kube.List(t.Context(), &left)).To(Succeed())
		g.Expect(left.Items).To(BeEmpty())
	})

	t.Run("nothing to delete with an output secret requested", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)

		err := finalize(h, true)

		g.Expect(status.Code(err)).To(Equal(codes.NotFound))
		g.Expect(status.Convert(err).Message()).To(Equal("no existing plan secrets found to delete"))
	})

	t.Run("output secret get fails", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarnessWith(t, []harnessOption{withInterceptors(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if key.Name == "app-outputs" {
					return errors.New("get boom")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		})}, seededPlan(), outputSecret("app-outputs"))

		err := finalize(h, true)

		g.Expect(status.Code(err)).To(Equal(codes.Internal))
		g.Expect(status.Convert(err).Message()).To(Equal("get boom"))
	})

	t.Run("output secret delete fails", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarnessWith(t, []harnessOption{withInterceptors(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if obj.GetName() == "app-outputs" {
					return errors.New("output delete boom")
				}
				return c.Delete(ctx, obj, opts...)
			},
		})}, seededPlan(), outputSecret("app-outputs"))

		err := finalize(h, true)

		g.Expect(status.Code(err)).To(Equal(codes.Internal))
		g.Expect(status.Convert(err).Message()).To(Equal("output delete boom"))
	})

	// A runner that may not read or delete secrets is told so with
	// PermissionDenied, which the controller reports without blocking
	// deletion, whichever call is refused.
	t.Run("forbidden", func(t *testing.T) {
		forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", errors.New("no access"))
		calls := map[string]interceptor.Funcs{
			"list plan secrets": {
				List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
					return forbidden
				},
			},
			"delete plan secret": {
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if obj.GetName() == "app-outputs" {
						return c.Delete(ctx, obj, opts...)
					}
					return forbidden
				},
			},
			"get output secret": {
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if key.Name == "app-outputs" {
						return forbidden
					}
					return c.Get(ctx, key, obj, opts...)
				},
			},
			"delete output secret": {
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if obj.GetName() == "app-outputs" {
						return forbidden
					}
					return c.Delete(ctx, obj, opts...)
				},
			},
		}
		for name, funcs := range calls {
			t.Run(name, func(t *testing.T) {
				g := NewWithT(t)
				h := newHarnessWith(t, []harnessOption{withInterceptors(funcs)}, seededPlan(), outputSecret("app-outputs"))

				err := finalize(h, true)

				g.Expect(status.Code(err)).To(Equal(codes.PermissionDenied))
				g.Expect(status.Convert(err).Message()).To(Equal(forbidden.Error()))
			})
		}
	})

	t.Run("success", func(t *testing.T) {
		g := NewWithT(t)
		secrets := planSecrets(t, "app", "default", "default", "plan-1", []byte("plan"))
		h := newHarness(t, secrets[0], outputSecret("app-outputs"))

		reply, err := h.Client.FinalizeSecrets(t.Context(), &runner.FinalizeSecretsRequest{
			Namespace:                "default",
			Name:                     "app",
			Workspace:                "default",
			HasSpecifiedOutputSecret: true,
			OutputSecretName:         "app-outputs",
		})

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(reply.Message).To(Equal("ok"))
		var left corev1.SecretList
		g.Expect(h.Kube.List(t.Context(), &left)).To(Succeed())
		g.Expect(left.Items).To(BeEmpty())
	})

	t.Run("legacy plan secret is deleted", func(t *testing.T) {
		g := NewWithT(t)
		// Old releases stored the plan without the plan labels.
		legacy := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "tfplan-default-app", Namespace: "default"}}
		h := newHarness(t, legacy)

		reply, err := h.Client.FinalizeSecrets(t.Context(), &runner.FinalizeSecretsRequest{
			Namespace: "default", Name: "app", Workspace: "default",
		})

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(reply.Message).To(Equal("ok"))
		var left corev1.SecretList
		g.Expect(h.Kube.List(t.Context(), &left)).To(Succeed())
		g.Expect(left.Items).To(BeEmpty())
	})

	t.Run("secrets of another plan are left alone", func(t *testing.T) {
		g := NewWithT(t)
		other := planSecrets(t, "other", "default", "default", "plan-1", []byte("plan"))[0]
		h := newHarness(t, seededPlan(), other)

		_, err := finalizeReply(h)

		g.Expect(err).NotTo(HaveOccurred())
		var left corev1.SecretList
		g.Expect(h.Kube.List(t.Context(), &left)).To(Succeed())
		g.Expect(left.Items).To(HaveLen(1))
		g.Expect(left.Items[0].Name).To(Equal(other.Name))
	})

	t.Run("no plan secrets still deletes the output secret", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t, outputSecret("app-outputs"))

		reply, err := h.Client.FinalizeSecrets(t.Context(), &runner.FinalizeSecretsRequest{
			Namespace:                "default",
			Name:                     "app",
			Workspace:                "default",
			HasSpecifiedOutputSecret: true,
			OutputSecretName:         "app-outputs",
		})

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(reply.Message).To(Equal("ok"))
		var left corev1.SecretList
		g.Expect(h.Kube.List(t.Context(), &left)).To(Succeed())
		g.Expect(left.Items).To(BeEmpty())
	})
}

func TestCharacterisation_WriteOutputs(t *testing.T) {
	g := NewWithT(t)
	h := newHarness(t)
	write := func(data map[string][]byte) *runner.WriteOutputsReply {
		reply, err := h.Client.WriteOutputs(t.Context(), &runner.WriteOutputsRequest{
			Namespace:   "default",
			Name:        "app",
			SecretName:  "app-outputs",
			Uuid:        "uid-1",
			Data:        data,
			Labels:      map[string]string{"l": "1"},
			Annotations: map[string]string{"a": "2"},
		})
		g.Expect(err).NotTo(HaveOccurred())
		return reply
	}
	key := client.ObjectKey{Namespace: "default", Name: "app-outputs"}

	reply := write(map[string][]byte{"a": []byte("1")})
	g.Expect(reply.Message).To(Equal("ok"))
	g.Expect(reply.Changed).To(BeTrue())
	var created corev1.Secret
	g.Expect(h.Kube.Get(t.Context(), key, &created)).To(Succeed())
	g.Expect(created.Type).To(Equal(corev1.SecretTypeOpaque))
	g.Expect(created.Labels).To(Equal(map[string]string{"l": "1"}))
	g.Expect(created.Annotations).To(Equal(map[string]string{"a": "2"}))
	g.Expect(created.OwnerReferences).To(HaveLen(1))
	owner := created.OwnerReferences[0]
	g.Expect(owner.APIVersion).To(Equal("infra.contrib.fluxcd.io/v1alpha2"))
	g.Expect(owner.Kind).To(Equal("Terraform"))
	g.Expect(owner.Name).To(Equal("app"))
	g.Expect(string(owner.UID)).To(Equal("uid-1"))
	g.Expect(owner.Controller).NotTo(BeNil())
	g.Expect(*owner.Controller).To(BeTrue())

	g.Expect(write(map[string][]byte{"a": []byte("1")}).Changed).To(BeFalse())

	g.Expect(write(map[string][]byte{"a": []byte("2")}).Changed).To(BeTrue())
	var updated corev1.Secret
	g.Expect(h.Kube.Get(t.Context(), key, &updated)).To(Succeed())
	g.Expect(updated.Data).To(Equal(map[string][]byte{"a": []byte("2")}))
	// The update keeps whatever owner the secret already had; it adds none.
	g.Expect(updated.OwnerReferences).To(Equal(created.OwnerReferences))
}

func TestCharacterisation_WriteOutputs_UpdateAddsNoOwner(t *testing.T) {
	g := NewWithT(t)
	// A secret that already exists without an owner stays without one.
	h := newHarness(t, outputSecret("app-outputs"))

	reply, err := h.Client.WriteOutputs(t.Context(), &runner.WriteOutputsRequest{
		Namespace:  "default",
		Name:       "app",
		SecretName: "app-outputs",
		Uuid:       "uid-1",
		Data:       map[string][]byte{"k": []byte("new")},
	})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(reply.Changed).To(BeTrue())
	var got corev1.Secret
	g.Expect(h.Kube.Get(t.Context(), client.ObjectKey{Namespace: "default", Name: "app-outputs"}, &got)).To(Succeed())
	g.Expect(got.Data).To(Equal(map[string][]byte{"k": []byte("new")}))
	g.Expect(got.OwnerReferences).To(BeEmpty())
}

func TestCharacterisation_LoadTFPlan(t *testing.T) {
	setupWith := func(t *testing.T, tf *infrav1.Terraform, objs ...client.Object) (*harness, string, string) {
		h := newHarnessWith(t, nil, objs...)
		wd := t.TempDir()
		id := h.newSession(t, tf, newStubTerraform(t).Path(), wd)
		return h, id, wd
	}
	setup := func(t *testing.T, objs ...client.Object) (*harness, string, string) {
		return setupWith(t, newTerraform("app"), objs...)
	}
	loadReply := func(h *harness, id string, disable bool) (*runner.LoadTFPlanReply, error) {
		return h.Client.LoadTFPlan(t.Context(), &runner.LoadTFPlanRequest{
			TfInstance:               id,
			Name:                     "app",
			Namespace:                "default",
			PendingPlan:              "plan-1",
			BackendCompletelyDisable: disable,
		})
	}
	load := func(h *harness, id string, disable bool) error {
		_, err := loadReply(h, id, disable)
		return err
	}

	t.Run("matching annotation writes the plan", func(t *testing.T) {
		g := NewWithT(t)
		h, id, wd := setup(t, planSecrets(t, "app", "default", "default", "plan-1", []byte("the plan"))[0])

		reply, err := loadReply(h, id, false)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(reply.Message).To(Equal("ok"))
		g.Expect(filepath.Join(wd, "tfplan")).To(BeAnExistingFile())
		got, err := os.ReadFile(filepath.Join(wd, "tfplan"))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(string(got)).To(Equal("the plan"))
	})

	t.Run("mismatched annotation", func(t *testing.T) {
		g := NewWithT(t)
		s := planSecrets(t, "app", "default", "default", "plan-other", []byte("the plan"))[0]
		h, id, wd := setup(t, s)

		err := load(h, id, false)

		g.Expect(status.Code(err)).To(Equal(codes.Internal))
		g.Expect(status.Convert(err).Message()).To(Equal(
			"pending plan plan-1 does not match secret " + s.Name + " (plan-other)"))
		g.Expect(filepath.Join(wd, "tfplan")).NotTo(BeAnExistingFile())
	})

	t.Run("no secrets", func(t *testing.T) {
		g := NewWithT(t)
		h, id, _ := setup(t)

		err := load(h, id, false)

		g.Expect(status.Code(err)).To(Equal(codes.Internal))
		g.Expect(status.Convert(err).Message()).To(Equal("no plan secrets found for plan plan-1"))
	})

	t.Run("backend completely disabled", func(t *testing.T) {
		g := NewWithT(t)
		h, id, wd := setup(t)

		reply, err := loadReply(h, id, true)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(reply.Message).To(Equal("ok"))
		g.Expect(filepath.Join(wd, "tfplan")).NotTo(BeAnExistingFile())
	})

	t.Run("force writes the plan despite a mismatched annotation", func(t *testing.T) {
		g := NewWithT(t)
		tf := newTerraform("app")
		tf.Spec.Force = true
		h, id, wd := setupWith(t, tf, planSecrets(t, "app", "default", "default", "plan-other", []byte("the plan"))[0])

		reply, err := loadReply(h, id, false)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(reply.Message).To(Equal("ok"))
		got, err := os.ReadFile(filepath.Join(wd, "tfplan"))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(string(got)).To(Equal("the plan"))
	})
}
