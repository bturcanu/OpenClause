package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/types"
)

func TestWaitForApprovalThenExecute_DoesNotRetryPermanentConflict(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/v1/toolcalls/evt-1/execute" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(types.APIError{
			Code:      "CONFLICT",
			Message:   "event does not require approval execution",
			Retryable: false,
		})
	}))
	t.Cleanup(srv.Close)

	client := New(srv.URL, "test-key")
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	_, err := client.WaitForApprovalThenExecute(ctx, "evt-1", time.Millisecond)
	if err == nil {
		t.Fatal("expected error for permanent conflict")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected exactly 1 execute attempt, got %d", got)
	}
}

func TestWaitForApprovalThenExecute_RetriesAwaitingApprovalConflict(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(types.APIError{
				Code:      "CONFLICT",
				Message:   "awaiting approval",
				Retryable: false,
			})
			return
		}

		_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
			EventID:  "exec-1",
			Decision: types.DecisionAllow,
			Result:   &types.ExecutionResult{Status: "success"},
		})
	}))
	t.Cleanup(srv.Close)

	client := New(srv.URL, "test-key")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	resp, err := client.WaitForApprovalThenExecute(ctx, "evt-1", time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.EventID != "exec-1" || resp.Result == nil || resp.Result.Status != "success" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 execute attempts, got %d", got)
	}
}
