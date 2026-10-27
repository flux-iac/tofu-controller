package controllers

import (
	"context"
	"reflect"
	"strings"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	"github.com/hashicorp/terraform-exec/tfexec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestOutputsMayBeDriftedDoesNotMutateSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "out", Namespace: "ns"},
		Data:       map[string][]byte{"port": []byte("1"), "foo": []byte("a")},
	}
	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{Name: "tf", Namespace: "ns"},
		Spec: infrav1.TerraformSpec{WriteOutputsToSecret: &infrav1.WriteOutputsToSecretSpec{
			Name:    "out",
			Outputs: []string{"port", "foo:a_alias"},
		}},
	}
	r := newFakeReconciler(fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build())

	if _, err := r.outputsMayBeDrifted(context.Background(), tf); err != nil {
		t.Fatal(err)
	}
	want := []string{"port", "foo:a_alias"}
	if got := tf.Spec.WriteOutputsToSecret.Outputs; !reflect.DeepEqual(got, want) {
		t.Fatalf("spec outputs mutated: got %v, want %v", got, want)
	}

	tf.Spec.WriteOutputsToSecret.Outputs = nil
	tf.Status.AvailableOutputs = []string{"port", "foo"}
	if _, err := r.outputsMayBeDrifted(context.Background(), tf); err != nil {
		t.Fatal(err)
	}
	if got := tf.Status.AvailableOutputs; !reflect.DeepEqual(got, []string{"port", "foo"}) {
		t.Fatalf("status outputs mutated: got %v", got)
	}
}

func TestOutputsWrittenEventReason(t *testing.T) {
	obj := fakeTerraform("outputs-event")
	obj.Spec.WriteOutputsToSecret = &infrav1.WriteOutputsToSecretSpec{Name: "out"}
	c, patcher := newFakePatcher(t, obj)
	r := newFakeReconciler(c)
	fr := newFakeRunner()
	fr.replies["WriteOutputs"] = &runner.WriteOutputsReply{Message: "ok", Changed: true}
	s := &runnerSession{client: fr, patcher: patcher, revision: "rev", instanceID: "loop-1"}

	outputs := map[string]tfexec.OutputMeta{"foo": {Type: []byte(`"string"`), Value: []byte(`"bar"`)}}
	if err := r.writeOutput(context.Background(), s, obj, outputs); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-r.EventRecorder.(*events.FakeRecorder).Events:
		if !strings.Contains(e, "TerraformOutputsWritten") || strings.Contains(e, "OutputsWritingFailed") {
			t.Fatalf("event %q: want reason TerraformOutputsWritten", e)
		}
	default:
		t.Fatal("no event recorded")
	}
}
