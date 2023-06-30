package planid

import (
	"fmt"
	"strings"
)

// getPlanIDv0 parses old revision format: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb
func getPlanIDv0(revision string) string {
	parts := strings.Split(revision, "/")
	if len(parts) != 2 {
		if len(revision) > 10 {
			hash := revision[:10]
			return "plan-" + hash
		}
		return "plan-" + revision
	}

	branch := parts[0]
	hash := parts[1]
	if len(hash) > 10 {
		hash = hash[:10]
	}

	planID := "plan-" + branch + "-" + hash
	return planID
}

// GetPlanID parses revision in ${branch}@${algo}:${hash}
// to plan ID is plan-${branch}-${hash 10 digits}
func GetPlanID(revision string) string {
	parts := strings.Split(revision, "@")
	if len(parts) != 2 {
		return getPlanIDv0(revision)
	}

	branch := parts[0]

	// parts[1] is now "${algo}:${hash}"
	hashParts := strings.Split(parts[1], ":")
	hash := hashParts[1]

	if len(hash) > 10 {
		hash = hash[:10]
	}

	planID := "plan-" + branch + "-" + hash
	return planID
}

func GetApproveMessage(planId string, message string) string {
	approveMessage := fmt.Sprintf("%s: set approvePlan: \"%s\" to approve this plan.", message, planId)
	return approveMessage
}
