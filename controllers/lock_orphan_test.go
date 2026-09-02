package controllers

import (
	"context"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/runtime/conditions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestParseTerraformLockCreated(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want time.Time
	}{
		{
			name: "fractional seconds and utc offset",
			raw: `error acquiring the state lock: Lock Info:
  ID:        fff6a1aa-c879-2e17-d197-1418368e715d
  Operation: OperationTypeApply
  Who:       runner@example-tf-runner
  Created:   2026-09-01 08:58:05.393443698 +0000 UTC
`,
			want: time.Date(2026, 9, 1, 8, 58, 5, 393443698, time.UTC),
		},
		{
			name: "non-utc offset",
			raw:  "Created:   2026-09-01 10:58:05.393443698 +0200 CEST\n",
			want: time.Date(2026, 9, 1, 8, 58, 5, 393443698, time.UTC),
		},
		{
			name: "whole seconds with zone",
			raw:  "Created:   2026-09-01 08:58:05 +0000 UTC",
			want: time.Date(2026, 9, 1, 8, 58, 5, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseTerraformLockCreated(tt.raw)
			if !ok {
				t.Fatal("expected to parse Created")
			}
			if !got.Equal(tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
			if got.Nanosecond() != tt.want.Nanosecond() {
				t.Fatalf("got nanoseconds %d want %d", got.Nanosecond(), tt.want.Nanosecond())
			}
		})
	}
}

func TestParseTerraformLockCreatedMissing(t *testing.T) {
	if _, ok := parseTerraformLockCreated("plain error"); ok {
		t.Fatal("expected no parse")
	}
}

func TestLockConditionForPlanErrorOrphaned(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := infrav1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	lockCreated := time.Date(2026, 9, 1, 8, 58, 5, 393443698, time.UTC)
	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "flux-system"},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "example-tf-runner",
			Namespace:         "flux-system",
			CreationTimestamp: metav1.NewTime(lockCreated.Add(48 * time.Minute)),
		},
	}
	r := &TerraformReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build(),
	}
	raw := `error acquiring the state lock: Lock Info:
  ID:        fff6a1aa-c879-2e17-d197-1418368e715d
  Operation: OperationTypeApply
  Who:       runner@example-tf-runner
  Created:   2026-09-01 08:58:05.393443698 +0000 UTC
`
	out := r.lockConditionForPlanError(context.Background(), tf, "fff6a1aa-c879-2e17-d197-1418368e715d", raw)
	c := conditions.Get(out, infrav1.ConditionTypeStateLocked)
	if c == nil {
		t.Fatal("expected StateLocked condition")
	}
	if c.Reason != infrav1.TFExecLockOrphanedReason {
		t.Fatalf("got reason %q want %q", c.Reason, infrav1.TFExecLockOrphanedReason)
	}
}

func TestLockConditionForPlanErrorLiveLock(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := infrav1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	lockCreated := time.Date(2026, 9, 1, 8, 58, 5, 393443698, time.UTC)
	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "flux-system"},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "example-tf-runner",
			Namespace:         "flux-system",
			CreationTimestamp: metav1.NewTime(lockCreated.Add(-time.Minute)),
		},
	}
	r := &TerraformReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build(),
	}
	raw := `error acquiring the state lock: Lock Info:
  ID:        fff6a1aa-c879-2e17-d197-1418368e715d
  Created:   2026-09-01 08:58:05.393443698 +0000 UTC
`
	out := r.lockConditionForPlanError(context.Background(), tf, "fff6a1aa-c879-2e17-d197-1418368e715d", raw)
	c := conditions.Get(out, infrav1.ConditionTypeStateLocked)
	if c == nil {
		t.Fatal("expected StateLocked condition")
	}
	if c.Reason != infrav1.TFExecLockHeldReason {
		t.Fatalf("got reason %q want %q", c.Reason, infrav1.TFExecLockHeldReason)
	}
}

func TestParseTerraformLockCreatedSameSecondUsesNanoseconds(t *testing.T) {
	created, ok := parseTerraformLockCreated("Created:   2026-09-01 08:58:05.393443698 +0000 UTC")
	if !ok {
		t.Fatal("expected to parse Created")
	}
	if created.Nanosecond() != 393443698 {
		t.Fatalf("got nanoseconds %d want 393443698", created.Nanosecond())
	}

	// Kubernetes metav1.Time is second-precision after an API round-trip, but
	// lock Created keeps nanoseconds. Truncating Created to whole seconds would
	// make an earlier same-second pod look like it started after the lock.
	earlier := time.Date(2026, 9, 1, 8, 58, 5, 100000000, time.UTC)
	later := time.Date(2026, 9, 1, 8, 58, 5, 500000000, time.UTC)
	if earlier.After(created) {
		t.Fatal("pod created earlier in the same second must not classify as orphaned")
	}
	if !later.After(created) {
		t.Fatal("pod created later in the same second must classify as orphaned")
	}
}
