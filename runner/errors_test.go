package runner

import (
	"context"
	"errors"
	"fmt"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToStatus(t *testing.T) {
	lock := &StateLockError{
		originalError: errors.New("locked"),
		ID:            "abc",
		Path:          "p",
		Operation:     "OperationTypePlan",
		Who:           "me@host",
		Version:       "1.6.0",
		Created:       "now",
	}
	notFound := apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, "s")
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", errors.New("no list"))
	unauthorized := apierrors.NewUnauthorized("bad token")
	preset := status.Error(codes.DeadlineExceeded, "too slow")

	tests := []struct {
		name       string
		err        error
		wantNil    bool
		wantCode   codes.Code
		wantMsg    string
		wantDetail *StateLock
	}{
		{name: "nil", err: nil, wantNil: true},
		{name: "existing status is kept", err: preset, wantCode: codes.DeadlineExceeded, wantMsg: "too slow"},
		{name: "state lock with id", err: lock, wantCode: codes.Aborted, wantMsg: lock.Error(), wantDetail: lock.Detail()},
		{
			name:       "wrapped state lock",
			err:        fmt.Errorf("plan: %w", lock),
			wantCode:   codes.Aborted,
			wantMsg:    "plan: " + lock.Error(),
			wantDetail: lock.Detail(),
		},
		{
			name:     "state lock without id",
			err:      &StateLockError{originalError: errors.New("no id")},
			wantCode: codes.Internal,
			wantMsg:  "error acquiring the state lock: no id",
		},
		{
			name:     "interrupted",
			err:      &InterruptedError{Reason: "context canceled", Err: errors.New("signal: interrupt")},
			wantCode: codes.Canceled,
			wantMsg:  "terraform interrupted (context canceled): signal: interrupt",
		},
		{
			name:     "session not initialised",
			err:      &SessionError{Requested: "a"},
			wantCode: codes.FailedPrecondition,
			wantMsg:  "terraform session error: instance id is empty, expected 'a'",
		},
		{
			name:     "session mismatch",
			err:      &SessionError{Requested: "a", Current: "b"},
			wantCode: codes.FailedPrecondition,
			wantMsg:  "terraform session mismatch: requested instance id 'a' does not match the current instance 'b'",
		},
		{
			name:     "wrapped session error",
			err:      fmt.Errorf("x: %w", &SessionError{Requested: "a", Current: "b"}),
			wantCode: codes.FailedPrecondition,
			wantMsg:  "x: terraform session mismatch: requested instance id 'a' does not match the current instance 'b'",
		},
		{
			name:       "not locked",
			err:        &NotLockedError{ID: "abc", Err: errors.New("no such lock")},
			wantCode:   codes.FailedPrecondition,
			wantMsg:    "no such lock",
			wantDetail: &StateLock{Id: "abc"},
		},
		{
			name:     "invalid input",
			err:      &InvalidInputError{Err: errors.New("bad path")},
			wantCode: codes.InvalidArgument,
			wantMsg:  "bad path",
		},
		{name: "kubernetes not found", err: notFound, wantCode: codes.NotFound, wantMsg: notFound.Error()},
		{
			name:     "wrapped kubernetes not found",
			err:      fmt.Errorf("get: %w", notFound),
			wantCode: codes.NotFound,
			wantMsg:  "get: " + notFound.Error(),
		},
		{name: "kubernetes forbidden", err: forbidden, wantCode: codes.PermissionDenied, wantMsg: forbidden.Error()},
		{name: "wrapped kubernetes forbidden", err: fmt.Errorf("list: %w", forbidden), wantCode: codes.PermissionDenied, wantMsg: "list: " + forbidden.Error()},
		{name: "kubernetes unauthorized", err: unauthorized, wantCode: codes.PermissionDenied, wantMsg: unauthorized.Error()},
		{name: "other kubernetes error", err: apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, "s", errors.New("c")), wantCode: codes.Internal, wantMsg: `Operation cannot be fulfilled on secrets "s": c`},
		{name: "plain error", err: errors.New("boom"), wantCode: codes.Internal, wantMsg: "boom"},
		{name: "context canceled", err: context.Canceled, wantCode: codes.Canceled, wantMsg: "context canceled"},
		{name: "wrapped context canceled", err: fmt.Errorf("x: %w", context.Canceled), wantCode: codes.Canceled, wantMsg: "x: context canceled"},
		{name: "deadline exceeded", err: context.DeadlineExceeded, wantCode: codes.DeadlineExceeded, wantMsg: "context deadline exceeded"},
		{
			name:     "wrapped deadline exceeded",
			err:      fmt.Errorf("x: %w", context.DeadlineExceeded),
			wantCode: codes.DeadlineExceeded,
			wantMsg:  "x: context deadline exceeded",
		},
		{
			name:     "interrupted wins over a wrapped context error",
			err:      &InterruptedError{Reason: "context canceled", Err: context.Canceled},
			wantCode: codes.Canceled,
			wantMsg:  "terraform interrupted (context canceled): context canceled",
		},
		{name: "existing Unknown status", err: status.Error(codes.Unknown, "odd"), wantCode: codes.Internal, wantMsg: "odd"},
		{
			name:     "wrapped status is kept",
			err:      fmt.Errorf("get: %w", preset),
			wantCode: codes.DeadlineExceeded,
			wantMsg:  "get: rpc error: code = DeadlineExceeded desc = too slow",
		},
		{
			name:     "wrapped Unknown status",
			err:      fmt.Errorf("get: %w", status.Error(codes.Unknown, "odd")),
			wantCode: codes.Internal,
			wantMsg:  "get: rpc error: code = Unknown desc = odd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToStatus(tt.err)
			if tt.wantNil {
				assert.NoError(t, got)
				return
			}

			require.Error(t, got)
			st, ok := status.FromError(got)
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, st.Code())
			assert.Equal(t, tt.wantMsg, st.Message())

			if tt.wantDetail == nil {
				assert.Empty(t, st.Details())
				return
			}

			require.Len(t, st.Details(), 1)
			detail, ok := st.Details()[0].(*StateLock)
			require.True(t, ok)
			assert.Equal(t, tt.wantDetail.Id, detail.Id)
			assert.Equal(t, tt.wantDetail.Path, detail.Path)
			assert.Equal(t, tt.wantDetail.Operation, detail.Operation)
			assert.Equal(t, tt.wantDetail.Who, detail.Who)
			assert.Equal(t, tt.wantDetail.Version, detail.Version)
			assert.Equal(t, tt.wantDetail.Created, detail.Created)
		})
	}
}

func TestToStatusKeepsStatusIdentity(t *testing.T) {
	preset := status.Error(codes.Unavailable, "gone")
	assert.Equal(t, preset, ToStatus(preset))
}

func TestInterruptedErrorWithoutCause(t *testing.T) {
	assert.Equal(t, "terraform interrupted (shutting down)", (&InterruptedError{Reason: "shutting down"}).Error())
}

func TestErrorsUnwrap(t *testing.T) {
	inner := errors.New("inner")
	assert.ErrorIs(t, &InvalidInputError{Err: inner}, inner)
	assert.ErrorIs(t, &InterruptedError{Reason: "r", Err: inner}, inner)
	assert.ErrorIs(t, &NotLockedError{ID: "x", Err: inner}, inner)
	assert.ErrorIs(t, &StateLockError{originalError: inner, ID: "x"}, inner)
}
