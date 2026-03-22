package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

func TestSubmitBuildsRequestAndAutoFillsIdentifiers(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotAPIKey string
	var gotBody types.ToolCallRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("X-API-Key")
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
			EventID:  "evt-1",
			Decision: types.DecisionAllow,
		})
	}))
	t.Cleanup(srv.Close)

	client := New(srv.URL, "test-key")
	resp, err := client.Submit(context.Background(), types.ToolCallRequest{
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		Tool:     "slack",
		Action:   "msg.post",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if resp == nil || resp.EventID != "evt-1" {
		t.Fatalf("unexpected submit response: %+v", resp)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/toolcalls" {
		t.Fatalf("unexpected request routing: method=%s path=%s", gotMethod, gotPath)
	}
	if gotAPIKey != "test-key" {
		t.Fatalf("expected X-API-Key header, got %q", gotAPIKey)
	}
	if gotBody.TenantID != "tenant-1" || gotBody.AgentID != "agent-1" || gotBody.Tool != "slack" || gotBody.Action != "msg.post" {
		t.Fatalf("unexpected submit body: %+v", gotBody)
	}
	if gotBody.IdempotencyKey == "" || gotBody.TraceID == "" {
		t.Fatalf("expected submit to auto-fill idempotency and trace ids, got %+v", gotBody)
	}
}

func TestExecuteReturnsStructuredAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/toolcalls/evt-404/execute" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(types.APIError{
			Code:    "NOT_FOUND",
			Message: "event not found",
		})
	}))
	t.Cleanup(srv.Close)

	client := New(srv.URL, "test-key")
	_, err := client.Execute(context.Background(), "evt-404")
	if err == nil {
		t.Fatal("expected API error")
	}
	var apiErr *types.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected structured APIError, got %T %v", err, err)
	}
	if apiErr.HTTPCode != http.StatusNotFound || apiErr.Code != "NOT_FOUND" || apiErr.Message != "event not found" {
		t.Fatalf("unexpected APIError payload: %+v", apiErr)
	}
}
