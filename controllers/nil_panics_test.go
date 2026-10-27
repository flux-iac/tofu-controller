package controllers

import (
	"context"
	"errors"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	. "github.com/onsi/gomega"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestDestroyErrorDoesNotPanic checks that with the backend disabled and
// spec.destroy set, a Destroy error is handled before the (nil) reply is read.
func TestDestroyErrorDoesNotPanic(t *testing.T) {
	g := NewWithT(t)
	r, s, obj, fr, _, c := newLoggedPass(t, "destroy-error")
	obj.Spec.Destroy = true
	obj.Spec.BackendConfig = &infrav1.BackendConfigSpec{Disable: true}
	obj.Status.Plan.Pending = "plan-main-b8e362c206"
	s.tfInstance = "loop-1"
	fr.errs["Destroy"] = errors.New("boom")

	err := r.apply(context.Background(), s, obj)
	g.Expect(err).To(MatchError("error running Destroy: boom"))

	ready := apimeta.FindStatusCondition(stored(t, c, obj).Status.Conditions, meta.ReadyCondition)
	g.Expect(ready).NotTo(BeNil())
	g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(ready.Reason).To(Equal(infrav1.TFExecApplyFailedReason))
	g.Expect(ready.Message).To(Equal("error running Destroy: boom"))
}

// TestInventoryErrorDoesNotPanic checks that a GetInventory error in the inventory
// refresh is logged and the pass continues, leaving the inventory unset.
func TestInventoryErrorDoesNotPanic(t *testing.T) {
	g := NewWithT(t)
	r, s, obj, fr, _, _ := newLoggedPass(t, "inventory-error")
	obj.Spec.EnableInventory = true
	s.tfInstance = "loop-1"
	fr.errs["GetInventory"] = errors.New("boom")

	g.Expect(func() { r.syncInventory(context.Background(), s, obj) }).NotTo(Panic())
	g.Expect(obj.Status.Inventory).To(BeNil())
	g.Expect(fr.Calls()).To(Equal([]string{"GetInventory"}))
}
