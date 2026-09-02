package controllers

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

// terraformLockCreatedRe captures the remainder of the lock Created line so
// fractional seconds and a timezone/offset are not dropped.
var terraformLockCreatedRe = regexp.MustCompile(`Created:\s+(.+)`)

// terraformLockCreatedLayouts match tofu/terraform lock output. The first
// layout is time.Time.String(), which is what LockInfo typically prints.
var terraformLockCreatedLayouts = []string{
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999 -0700",
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
}

// parseTerraformLockCreated extracts the lock Created wall time from a
// terraform/tofu "Error acquiring the state lock" message, including
// sub-second precision and zone/offset when present.
func parseTerraformLockCreated(raw string) (time.Time, bool) {
	m := terraformLockCreatedRe.FindStringSubmatch(raw)
	if len(m) < 2 {
		return time.Time{}, false
	}
	value := strings.TrimSpace(m[1])
	for _, layout := range terraformLockCreatedLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
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
		"State lock %s looks orphaned: lock Created %s is before runner pod creationTimestamp %s",
		lockID,
		created.UTC().Format(time.RFC3339Nano),
		pod.CreationTimestamp.UTC().Format(time.RFC3339Nano),
	)
	ctrl.LoggerFrom(ctx).Info(msg)
	return infrav1.TerraformLockOrphaned(terraform, lockID, msg)
}
