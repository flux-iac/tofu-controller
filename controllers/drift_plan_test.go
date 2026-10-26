package controllers

import (
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPlanHasOnlyDataReads(t *testing.T) {
	read := &tfjson.ResourceChange{Mode: tfjson.DataResourceMode, Change: &tfjson.Change{Actions: tfjson.Actions{tfjson.ActionRead}}}
	for _, tt := range []struct {
		name string
		plan *tfjson.Plan
		want bool
	}{
		{name: "nil plan"},
		{name: "empty plan", plan: &tfjson.Plan{}},
		{name: "deferred check read", plan: &tfjson.Plan{ResourceChanges: []*tfjson.ResourceChange{read}}, want: true},
		{name: "read and unchanged resource", plan: &tfjson.Plan{ResourceChanges: []*tfjson.ResourceChange{read, {Mode: tfjson.ManagedResourceMode, Change: &tfjson.Change{Actions: tfjson.Actions{tfjson.ActionNoop}}}}}, want: true},
		{name: "read and output change", plan: &tfjson.Plan{ResourceChanges: []*tfjson.ResourceChange{read}, OutputChanges: map[string]*tfjson.Change{"value": {Actions: tfjson.Actions{tfjson.ActionUpdate}}}}},
		{name: "read and unchanged output", plan: &tfjson.Plan{ResourceChanges: []*tfjson.ResourceChange{read}, OutputChanges: map[string]*tfjson.Change{"value": {Actions: tfjson.Actions{tfjson.ActionNoop}}}}, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, planHasOnlyDataReads(tt.plan))
		})
	}
	for _, action := range []tfjson.Action{tfjson.ActionCreate, tfjson.ActionUpdate, tfjson.ActionDelete} {
		t.Run(string(action), func(t *testing.T) {
			plan := &tfjson.Plan{ResourceChanges: []*tfjson.ResourceChange{read, {Mode: tfjson.ManagedResourceMode, Change: &tfjson.Change{Actions: tfjson.Actions{action}}}}}
			assert.False(t, planHasOnlyDataReads(plan))
		})
	}
}
