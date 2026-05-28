package controllers

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Test A — pure unit: getLabelsAsHCL emits only the internal label, not CR labels.
func Test_000362_state_secret_label_hcl_uses_internal_label(t *testing.T) {
	Spec("getLabelsAsHCL should emit only the internal label.")
	g := NewWithT(t)
	uid := "abc-123"
	hcl := getLabelsAsHCL(map[string]string{
		infrav1.TFStateLabelKey: uid,
	}, 6)
	g.Expect(hcl).To(ContainSubstring(infrav1.TFStateLabelKey))
	g.Expect(hcl).To(ContainSubstring(uid))
	g.Expect(hcl).NotTo(ContainSubstring("app"))
	g.Expect(hcl).NotTo(ContainSubstring("env"))
}

// Test C — integration: migration patches a legacy Secret that has no internal label.
func Test_000362_state_secret_migration_patches_legacy_secret(t *testing.T) {
	Spec("migrateStateSecretLabel should patch a pre-fix Secret with the CR UID label.")
	g := NewWithT(t)
	ctx := t.Context()

	const namespace = "flux-system"
	const crName = "tc000362-migrate-legacy"
	const fakeUID = types.UID("uid-migrate-legacy")

	// Use an in-memory CR so the reconciler never picks it up (no finalizer issues).
	cr := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: namespace,
			UID:       fakeUID,
			Labels:    map[string]string{"app": "foo"},
		},
	}

	By("creating a legacy state Secret with CR labels but no internal label.")
	secretName := fmt.Sprintf("tfstate-default-%s", crName)
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels:    map[string]string{"app": "foo"},
		},
	}
	g.Expect(k8sClient.Create(ctx, &secret)).To(Succeed())
	defer waitResourceToBeDelete(g, &secret)

	By("calling migrateStateSecretLabel.")
	r := &TerraformReconciler{Client: k8sClient, FieldManager: "tf-controller"}
	g.Expect(r.migrateStateSecretLabel(ctx, cr)).To(Succeed())

	By("verifying the Secret gained the internal UID label.")
	var patched corev1.Secret
	g.Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretName}, &patched)).To(Succeed())
	g.Expect(patched.Labels[infrav1.TFStateLabelKey]).To(Equal(string(fakeUID)))
}

// Test D — integration: label change on CR does not affect an already-migrated Secret.
func Test_000362_state_secret_label_change_does_not_affect_migrated_secret(t *testing.T) {
	Spec("migrateStateSecretLabel should be a no-op when the Secret already has the UID label.")
	g := NewWithT(t)
	ctx := t.Context()

	const namespace = "flux-system"
	const crName = "tc000362-label-change"
	const fakeUID = types.UID("uid-label-change")

	cr := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: namespace,
			UID:       fakeUID,
			Labels:    map[string]string{"app": "old"},
		},
	}

	By("creating a state Secret that is already migrated (has the UID label).")
	secretName := fmt.Sprintf("tfstate-default-%s", crName)
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                   "old",
				infrav1.TFStateLabelKey: string(fakeUID),
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, &secret)).To(Succeed())
	defer waitResourceToBeDelete(g, &secret)

	By("simulating a CR label mutation to {app: new}.")
	cr.Labels = map[string]string{"app": "new"}

	By("calling migrateStateSecretLabel with the updated CR.")
	r := &TerraformReconciler{Client: k8sClient, FieldManager: "tf-controller"}
	g.Expect(r.migrateStateSecretLabel(ctx, cr)).To(Succeed())

	By("verifying the Secret UID label is unchanged.")
	var got corev1.Secret
	g.Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretName}, &got)).To(Succeed())
	g.Expect(got.Labels[infrav1.TFStateLabelKey]).To(Equal(string(fakeUID)))
}

// Test E — integration: no state Secret → migration is a no-op (NotFound path).
func Test_000362_state_secret_migration_no_secret_is_noop(t *testing.T) {
	Spec("migrateStateSecretLabel should return nil when no state Secret exists.")
	g := NewWithT(t)
	ctx := t.Context()

	const namespace = "flux-system"
	const crName = "tc000362-no-secret"

	cr := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: namespace,
			UID:       types.UID("uid-no-secret"),
		},
	}

	By("calling migrateStateSecretLabel with no state Secret present.")
	r := &TerraformReconciler{Client: k8sClient, FieldManager: "tf-controller"}
	g.Expect(r.migrateStateSecretLabel(ctx, cr)).To(Succeed())
}

// Test F — integration: calling migration twice is idempotent.
func Test_000362_state_secret_migration_is_idempotent(t *testing.T) {
	Spec("migrateStateSecretLabel should be idempotent — a second call is a no-op.")
	g := NewWithT(t)
	ctx := t.Context()

	const namespace = "flux-system"
	const crName = "tc000362-idempotent"
	const fakeUID = types.UID("uid-idempotent")

	cr := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: namespace,
			UID:       fakeUID,
		},
	}

	By("creating a legacy Secret without the internal label.")
	secretName := fmt.Sprintf("tfstate-default-%s", crName)
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels:    map[string]string{"legacy": "true"},
		},
	}
	g.Expect(k8sClient.Create(ctx, &secret)).To(Succeed())
	defer waitResourceToBeDelete(g, &secret)

	r := &TerraformReconciler{Client: k8sClient, FieldManager: "tf-controller"}

	By("calling migrateStateSecretLabel the first time.")
	g.Expect(r.migrateStateSecretLabel(ctx, cr)).To(Succeed())

	var afterFirst corev1.Secret
	g.Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretName}, &afterFirst)).To(Succeed())
	g.Expect(afterFirst.Labels[infrav1.TFStateLabelKey]).To(Equal(string(fakeUID)))

	By("calling migrateStateSecretLabel a second time.")
	g.Expect(r.migrateStateSecretLabel(ctx, cr)).To(Succeed())

	By("verifying the Secret labels are unchanged after the second call.")
	var afterSecond corev1.Secret
	g.Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretName}, &afterSecond)).To(Succeed())
	g.Expect(afterSecond.Labels[infrav1.TFStateLabelKey]).To(Equal(string(fakeUID)))
	g.Expect(afterSecond.ResourceVersion).To(Equal(afterFirst.ResourceVersion))
}
