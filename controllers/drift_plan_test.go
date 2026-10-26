package controllers

import (
	"errors"

	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsPlanMessageSizeError(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "no error"},
		{name: "receive limit", err: status.Error(codes.ResourceExhausted, "grpc: received message larger than max (500 vs. 100)"), want: true},
		{name: "send limit", err: status.Error(codes.ResourceExhausted, "grpc: trying to send message larger than max (500 vs. 100)"), want: true},
		{name: "other resource exhaustion", err: status.Error(codes.ResourceExhausted, "quota exceeded")},
		{name: "other status", err: status.Error(codes.Internal, "message larger than max")},
		{name: "plain error", err: errors.New("message larger than max")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isPlanMessageSizeError(tt.err))
		})
	}
}

func TestPlanHasOnlyDataReads(t *testing.T) {
	read := &tfjson.ResourceChange{Mode: tfjson.DataResourceMode, Change: &tfjson.Change{Actions: tfjson.Actions{tfjson.ActionRead}}}
	for _, tt := range []struct {
		name string
		plan *tfjson.Plan
		want bool
	}{
		{name: "nil plan"},
		{name: "empty plan", plan: &tfjson.Plan{}},
		{name: "read and pending move", plan: &tfjson.Plan{ResourceChanges: []*tfjson.ResourceChange{read, {Mode: tfjson.ManagedResourceMode, PreviousAddress: "example.old", Change: &tfjson.Change{Actions: tfjson.Actions{tfjson.ActionNoop}}}}}},
		{name: "read and pending import", plan: &tfjson.Plan{ResourceChanges: []*tfjson.ResourceChange{read, {Mode: tfjson.ManagedResourceMode, Change: &tfjson.Change{Actions: tfjson.Actions{tfjson.ActionNoop}, Importing: &tfjson.Importing{}}}}}},
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
