package runner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTerraformEmptyInstanceID(t *testing.T) {
	server := &TerraformRunnerServer{}

	req := &InitRequest{
		TfInstance: "71bd530c-ba53-45ee-9b2c-f054b53be16e",
	}

	_, err := server.Init(context.Background(), req)
	if err != nil {
		var emptyErr *TerraformSessionNotInitializedError
		assert.ErrorAs(t, err, &emptyErr)

		assert.Equal(t, req.TfInstance, emptyErr.RequestedInstanceID)
	}
}

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
