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
		json.NewEncoder(w).Encode(resp)
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
		json.NewEncoder(w).Encode(resp)
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
