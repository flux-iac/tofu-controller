package controllers

import (
	"errors"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

// errDriftDetected is returned by detectDrift when a drift is found. Match it with errors.Is.
var errDriftDetected = errors.New(infrav1.DriftDetectedReason)
