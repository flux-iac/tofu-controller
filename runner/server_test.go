package runner

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
	. "github.com/onsi/gomega"
)

func Test_printHumanReadablePlanIfEnabled(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	defer func() {
		g.Expect(os.Unsetenv("LOG_HUMAN_READABLE_PLAN")).Should(Succeed())
	}()

	var tfShowPlanFileRawCalled int
	expectedPlan := TFPlanName
	tfShowPlanFileRaw := func(ctx context.Context, planPath string, opts ...tfexec.ShowOption) (string, error) {
		g.Expect(planPath).To(Equal(expectedPlan))
		tfShowPlanFileRawCalled++
		return "", nil
	}

	// When plan is enabled, then it should be called once
	g.Expect(os.Setenv("LOG_HUMAN_READABLE_PLAN", "1")).Should(Succeed())
	g.Expect(printHumanReadablePlanIfEnabled(ctx, "", tfShowPlanFileRaw)).Should(Succeed())
	g.Expect(tfShowPlanFileRawCalled).To(Equal(1))

	// When the planName is non-empty, then it should use the planName
	expectedPlan = "foo"
	g.Expect(printHumanReadablePlanIfEnabled(ctx, expectedPlan, tfShowPlanFileRaw)).Should(Succeed())
	g.Expect(tfShowPlanFileRawCalled).To(Equal(2))

	// When it is disabled, then it should not be called
	expectedPlan = TFPlanName
	g.Expect(os.Setenv("LOG_HUMAN_READABLE_PLAN", "0")).Should(Succeed())
	g.Expect(printHumanReadablePlanIfEnabled(ctx, "", tfShowPlanFileRaw)).Should(Succeed())
	g.Expect(tfShowPlanFileRawCalled).To(Equal(2))

	// When tfShowPlanFileRaw fails, then it should return an error
	g.Expect(os.Setenv("LOG_HUMAN_READABLE_PLAN", "1")).Should(Succeed())
	tfShowPlanFileRaw = func(ctx context.Context, planPath string, opts ...tfexec.ShowOption) (string, error) {
		return "", fmt.Errorf("error")
	}

	g.Expect(printHumanReadablePlanIfEnabled(ctx, "", tfShowPlanFileRaw)).ShouldNot(Succeed())
}
