package controllers

import tfjson "github.com/hashicorp/terraform-json"

// planHasOnlyDataReads excludes deferred check reads without hiding resource or
// output changes. Keep the CLI result for plans with no reads at all.
func planHasOnlyDataReads(plan *tfjson.Plan) bool {
	if plan == nil {
		return false
	}
	hasRead := false
	for _, resource := range plan.ResourceChanges {
		if resource == nil || resource.Change == nil {
			return false
		}
		for _, action := range resource.Change.Actions {
			switch action {
			case tfjson.ActionNoop:
			case tfjson.ActionRead:
				if resource.Mode != tfjson.DataResourceMode {
					return false
				}
				hasRead = true
			default:
				return false
			}
		}
	}
	for _, output := range plan.OutputChanges {
		if output == nil {
			return false
		}
		for _, action := range output.Actions {
			if action != tfjson.ActionNoop {
				return false
			}
		}
	}
	return hasRead
}
