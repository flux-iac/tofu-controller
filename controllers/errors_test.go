package controllers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

func TestErrDriftDetectedIsMatchable(t *testing.T) {
	if !errors.Is(fmt.Errorf("x: %w", errDriftDetected), errDriftDetected) {
		t.Fatal("wrapped errDriftDetected must match with errors.Is")
	}
	if errDriftDetected.Error() != infrav1.DriftDetectedReason {
		t.Fatalf("errDriftDetected.Error() = %q, want %q", errDriftDetected.Error(), infrav1.DriftDetectedReason)
	}
}

func TestNoStringErrorComparisons(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, ".Error() ==") || strings.Contains(line, ".Error() !=") {
				t.Errorf("%s:%d compares an error string: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}
