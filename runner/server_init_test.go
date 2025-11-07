package runner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTerraformMismatchInstanceIDs(t *testing.T) {
	server := &TerraformRunnerServer{
		InstanceID: "51b32416-d76d-4720-b2ef-1c13996d3c4a",
	}

	req := &InitRequest{
		TfInstance: "b17126a3-faf1-4265-a828-06f130b8c841",
	}

	_, err := server.Init(context.Background(), req)
	if err != nil {
		var mismatchErr *TerraformSessionMismatchError
		assert.ErrorAs(t, err, &mismatchErr)

		assert.Equal(t, req.TfInstance, mismatchErr.RequestedInstanceID)
		assert.Equal(t, server.InstanceID, mismatchErr.CurrentInstanceID)
	}
}
