package runner

import (
	"context"
	"fmt"
	"os"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/hashicorp/terraform-exec/tfexec"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func TestTerraformEmptyInstanceID(t *testing.T) {
	server := &TerraformRunnerServer{}

	requested := "71bd530c-ba53-45ee-9b2c-f054b53be16e"

	_, err := server.checkSession(requested)

	var emptyErr *SessionError
	assert.ErrorAs(t, err, &emptyErr)
	assert.Equal(t, requested, emptyErr.Requested)
	assert.Empty(t, emptyErr.Current)
}

func TestTerraformMismatchInstanceIDs(t *testing.T) {
	current := "51b32416-d76d-4720-b2ef-1c13996d3c4a"
	server := &TerraformRunnerServer{current: &session{id: current}}

	requested := "b17126a3-faf1-4265-a828-06f130b8c841"

	_, err := server.checkSession(requested)

	var mismatchErr *SessionError
	assert.ErrorAs(t, err, &mismatchErr)
	assert.Equal(t, requested, mismatchErr.Requested)
	assert.Equal(t, current, mismatchErr.Current)
}

func TestTerraformMatchingInstanceID(t *testing.T) {
	s := &session{id: "51b32416-d76d-4720-b2ef-1c13996d3c4a"}
	server := &TerraformRunnerServer{current: s}

	got, err := server.checkSession(s.id)

	assert.NoError(t, err)
	assert.Same(t, s, got)
}

// TestAppendPlanSpecOptions asserts on the exact tfexec option values, not just
// their types: tfexec.Lock(false) and tfexec.Lock(true) compare unequal under
// assert.Equal (deep comparison of the option struct), so this distinguishes
// -lock=false from -lock=true rather than merely confirming a lock option exists.
func TestAppendPlanSpecOptions(t *testing.T) {
	tests := []struct {
		name     string
		plan     *infrav1.PlanSpec
		expected []tfexec.PlanOption
	}{
		{
			name:     "nil spec adds nothing",
			plan:     nil,
			expected: nil,
		},
		{
			name:     "empty spec adds nothing",
			plan:     &infrav1.PlanSpec{},
			expected: nil,
		},
		{
			name:     "lock disabled emits -lock=false",
			plan:     &infrav1.PlanSpec{Lock: ptr.To(false)},
			expected: []tfexec.PlanOption{tfexec.Lock(false)},
		},
		{
			name:     "lock enabled emits -lock=true",
			plan:     &infrav1.PlanSpec{Lock: ptr.To(true)},
			expected: []tfexec.PlanOption{tfexec.Lock(true)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := appendPlanSpecOptions(nil, tt.plan)
			assert.Equal(t, tt.expected, opts)
		})
	}
}

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
