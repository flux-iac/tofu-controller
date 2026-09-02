package controllers

import (
	"context"
	"fmt"
	"regexp"
	"time"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

var terraformLockCreatedRe = regexp.MustCompile(`Created:\s+(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`)

// parseTerraformLockCreated extracts the lock Created wall time from a
// terraform/tofu "Error acquiring the state lock" message.
func parseTerraformLockCreated(raw string) (time.Time, bool) {
	m := terraformLockCreatedRe.FindStringSubmatch(raw)
	if len(m) < 2 {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", m[1], time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func (r *TerraformReconciler) lockConditionForPlanError(ctx context.Context, terraform *infrav1.Terraform, lockID, rawErr string) *infrav1.Terraform {
	locked := infrav1.TerraformStateLocked(terraform, lockID, fmt.Sprintf("Terraform Locked with Lock Identifier: %s", lockID))
	created, ok := parseTerraformLockCreated(rawErr)
	if !ok {
		return locked
	}
	var pod corev1.Pod
	if err := r.Get(ctx, getRunnerPodObjectKey(terraform), &pod); err != nil {
		return locked
	}
	if pod.CreationTimestamp.Time.IsZero() {
		return locked
	}
	if !pod.CreationTimestamp.Time.After(created) {
		return locked
	}
	msg := fmt.Sprintf(
		"State lock %s looks orphaned: lock Created %s is before runner pod start %s",
		lockID,
		created.UTC().Format(time.RFC3339),
		pod.CreationTimestamp.UTC().Format(time.RFC3339),
	)
	ctrl.LoggerFrom(ctx).Info(msg)
	return infrav1.TerraformLockOrphaned(terraform, lockID, msg)
}
