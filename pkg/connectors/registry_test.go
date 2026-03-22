package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRegistry_ExecSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ExecResponse{Status: "success", OutputJSON: json.RawMessage(`{"ok":true}`)}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	reg := NewRegistry()
	reg.Register("test", srv.URL)

	resp, err := reg.Exec(context.Background(), ExecRequest{Tool: "test", Action: "do"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "success" {
		t.Errorf("expected success, got %s", resp.Status)
	}
}

func TestRegistry_UnregisteredTool(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Exec(context.Background(), ExecRequest{Tool: "unknown", Action: "do"})
	if err == nil {
		t.Fatal("expected error for unregistered tool")
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewRegistry()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ExecResponse{Status: "success"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	reg.Register("test", srv.URL)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = reg.Exec(context.Background(), ExecRequest{Tool: "test", Action: "do"})
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestRegistry_SetTimeout(t *testing.T) {
	reg := NewRegistry()
	reg.SetTimeout(5 * time.Second)
}

func TestRegistry_ListAll_SortsAndDedupesCatalog(t *testing.T) {
	reg := NewRegistry()
	reg.Register("zeta", "http://remote.example", "write", "read", "write")
	reg.RegisterBuiltin("alpha", []string{"fetch", "fetch", "list"}, func(context.Context, ExecRequest) ExecResponse {
		return ExecResponse{Status: "success"}
	})
	// Remote registrations should win for duplicate tool names.
	reg.RegisterBuiltin("zeta", []string{"builtin-only"}, func(context.Context, ExecRequest) ExecResponse {
		return ExecResponse{Status: "success"}
	})

	got := reg.ListAll()
	if len(got) != 2 {
		t.Fatalf("expected 2 connectors, got %+v", got)
	}
	if got[0].Name != "alpha" || got[0].Type != "builtin" {
		t.Fatalf("expected alpha builtin first, got %+v", got[0])
	}
	if len(got[0].Actions) != 2 || got[0].Actions[0] != "fetch" || got[0].Actions[1] != "list" {
		t.Fatalf("expected alpha actions to be sorted and deduped, got %+v", got[0].Actions)
	}
	if got[1].Name != "zeta" || got[1].Type != "remote" {
		t.Fatalf("expected zeta remote second, got %+v", got[1])
	}
	if len(got[1].Actions) != 2 || got[1].Actions[0] != "read" || got[1].Actions[1] != "write" {
		t.Fatalf("expected zeta actions to be sorted and deduped, got %+v", got[1].Actions)
	}
}
