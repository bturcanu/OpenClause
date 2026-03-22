package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/types"
)

type fakeExportStore struct {
	csvErr        error
	bundleErr     error
	csvWriteBytes string
	events        []console.EventListItem
	count         int
	countErr      error
	since         time.Time
	until         time.Time
}

func (f *fakeExportStore) ExportEventsCSV(_ context.Context, _ string, _ time.Time, _ time.Time, w io.Writer) error {
	if f.csvWriteBytes != "" {
		_, _ = w.Write([]byte(f.csvWriteBytes))
	}
	return f.csvErr
}

func (f *fakeExportStore) CountEventsInRange(_ context.Context, _ string, since, until time.Time) (int, error) {
	f.since = since
	f.until = until
	if f.countErr != nil {
		return 0, f.countErr
	}
	return f.count, nil
}

func (f *fakeExportStore) ListEventsInRange(_ context.Context, _ string, since, until time.Time, _ int) ([]console.EventListItem, error) {
	f.since = since
	f.until = until
	if f.bundleErr != nil {
		return nil, f.bundleErr
	}
	return f.events, nil
}

func newTestConsoleAPI(exportStore exportEventsStore) *ConsoleAPI {
	return &ConsoleAPI{
		log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		exportStore: exportStore,
	}
}

func TestHandleExportEventsCSV_ExportErrorReturnsAPIErrorWithoutPartialCSV(t *testing.T) {
	const tenantID = "tenant-1"
	since := "2020-01-01T00:00:00Z"
	until := "2020-01-02T00:00:00Z"

	api := newTestConsoleAPI(&fakeExportStore{
		csvErr:        errors.New("boom"),
		csvWriteBytes: "event_id,tenant_id\npartial\n",
	})

	claims := &console.JWTClaims{
		Roles: []string{"platform_admin"},
	}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(
		http.MethodGet,
		"/admin/events/export/csv?tenant_id="+tenantID+"&since="+since+"&until="+until,
		nil,
	).WithContext(ctx)

	rr := httptest.NewRecorder()
	api.handleExportEventsCSV(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	var apiErr types.APIError
	if err := json.Unmarshal(rr.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("expected JSON APIError, got decode error=%v body=%s", err, rr.Body.String())
	}
	if apiErr.Code != "INTERNAL_ERROR" {
		t.Fatalf("expected INTERNAL_ERROR, got %s", apiErr.Code)
	}
	if strings.Contains(rr.Body.String(), "event_id") {
		t.Fatalf("expected no partial CSV output; got body=%s", rr.Body.String())
	}
}

func TestHandleExportBundle_ListEventsErrorReturnsAPIError(t *testing.T) {
	const tenantID = "tenant-1"
	since := "2020-01-01T00:00:00Z"
	until := "2020-01-02T00:00:00Z"

	api := newTestConsoleAPI(&fakeExportStore{
		count:     1,
		bundleErr: errors.New("boom"),
	})

	claims := &console.JWTClaims{
		Roles: []string{"platform_admin"},
	}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(
		http.MethodGet,
		"/admin/reports/export/bundle?tenant_id="+tenantID+"&since="+since+"&until="+until,
		nil,
	).WithContext(ctx)

	rr := httptest.NewRecorder()
	api.handleExportBundle(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	var apiErr types.APIError
	if err := json.Unmarshal(rr.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("expected JSON APIError, got decode error=%v body=%s", err, rr.Body.String())
	}
	if apiErr.Code != "INTERNAL_ERROR" {
		t.Fatalf("expected INTERNAL_ERROR, got %s", apiErr.Code)
	}
}

func TestHandleExportBundle_TenantDenySentinelReturns403(t *testing.T) {
	api := newTestConsoleAPI(&fakeExportStore{})

	// Non-platform-admin, empty tenant claim -> tenantScope returns tenantDenySentinel.
	claims := &console.JWTClaims{
		Roles:  []string{},
		Tenant: "",
	}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(
		http.MethodGet,
		"/admin/reports/export/bundle?tenant_id=tenant-1&since=2020-01-01T00:00:00Z&until=2020-01-02T00:00:00Z",
		nil,
	).WithContext(ctx)

	rr := httptest.NewRecorder()
	api.handleExportBundle(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
	var apiErr types.APIError
	if err := json.Unmarshal(rr.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("expected JSON APIError, got decode error=%v body=%s", err, rr.Body.String())
	}
	if apiErr.Code != "FORBIDDEN" {
		t.Fatalf("expected FORBIDDEN, got %s", apiErr.Code)
	}
}

type failingWriter struct{}

func (f failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestEncodeBundleJSON_PropagatesWriterErrors(t *testing.T) {
	bundle := map[string]any{
		"version": "1.0",
	}
	if err := encodeBundleJSON(failingWriter{}, bundle); err == nil {
		t.Fatal("expected encoder error, got nil")
	}
}

func TestHandleExportEventsCSV_MissingTenantIDReturnsStructuredAPIError(t *testing.T) {
	const since = "2020-01-01T00:00:00Z"
	const until = "2020-01-02T00:00:00Z"

	api := newTestConsoleAPI(&fakeExportStore{})

	claims := &console.JWTClaims{
		Roles: []string{"platform_admin"},
	}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(
		http.MethodGet,
		"/admin/events/export/csv?since="+since+"&until="+until,
		nil,
	).WithContext(ctx)

	rr := httptest.NewRecorder()
	api.handleExportEventsCSV(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}

	var apiErr types.APIError
	if err := json.Unmarshal(rr.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("expected JSON APIError, got decode error=%v body=%s", err, rr.Body.String())
	}
	if apiErr.Code != "BAD_REQUEST" {
		t.Fatalf("expected BAD_REQUEST, got %s", apiErr.Code)
	}
	if apiErr.Message != "tenant_id required for CSV export" {
		t.Fatalf("expected message %q, got %q", "tenant_id required for CSV export", apiErr.Message)
	}
	if apiErr.Retryable {
		t.Fatalf("expected retryable=false")
	}
}

func TestHandleExportBundle_MissingTenantIDReturnsStructuredAPIError(t *testing.T) {
	const since = "2020-01-01T00:00:00Z"
	const until = "2020-01-02T00:00:00Z"

	api := newTestConsoleAPI(&fakeExportStore{})

	claims := &console.JWTClaims{
		Roles: []string{"platform_admin"},
	}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(
		http.MethodGet,
		"/admin/reports/export/bundle?since="+since+"&until="+until,
		nil,
	).WithContext(ctx)

	rr := httptest.NewRecorder()
	api.handleExportBundle(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}

	var apiErr types.APIError
	if err := json.Unmarshal(rr.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("expected JSON APIError, got decode error=%v body=%s", err, rr.Body.String())
	}
	if apiErr.Code != "BAD_REQUEST" {
		t.Fatalf("expected BAD_REQUEST, got %s", apiErr.Code)
	}
	if apiErr.Message != "tenant_id required" {
		t.Fatalf("expected message %q, got %q", "tenant_id required", apiErr.Message)
	}
	if apiErr.Retryable {
		t.Fatalf("expected retryable=false")
	}
}

func TestHandleExportBundle_UsesUntilQueryParameter(t *testing.T) {
	const tenantID = "tenant-1"
	const since = "2020-01-01T00:00:00Z"
	const until = "2020-01-02T03:04:05Z"

	store := &fakeExportStore{
		count:  1,
		events: []console.EventListItem{{EventID: "evt-1"}},
	}
	api := newTestConsoleAPI(store)

	claims := &console.JWTClaims{Roles: []string{"platform_admin"}}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(
		http.MethodGet,
		"/admin/reports/export/bundle?tenant_id="+tenantID+"&since="+since+"&until="+until,
		nil,
	).WithContext(ctx)

	rr := httptest.NewRecorder()
	api.handleExportBundle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := store.until.Format(time.RFC3339); got != until {
		t.Fatalf("expected until %q, got %q", until, got)
	}
	if got := store.since.Format(time.RFC3339); got != since {
		t.Fatalf("expected since %q, got %q", since, got)
	}
}

func TestHandleExportBundle_RejectsOversizedRanges(t *testing.T) {
	api := newTestConsoleAPI(&fakeExportStore{count: 10001})

	claims := &console.JWTClaims{Roles: []string{"platform_admin"}}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(
		http.MethodGet,
		"/admin/reports/export/bundle?tenant_id=tenant-1&since=2020-01-01T00:00:00Z&until=2020-01-02T00:00:00Z",
		nil,
	).WithContext(ctx)

	rr := httptest.NewRecorder()
	api.handleExportBundle(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}

	var apiErr types.APIError
	if err := json.Unmarshal(rr.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("expected JSON APIError, got decode error=%v body=%s", err, rr.Body.String())
	}
	if apiErr.Code != "BAD_REQUEST" {
		t.Fatalf("expected BAD_REQUEST, got %s", apiErr.Code)
	}
	if !strings.Contains(apiErr.Message, "bundle export range too large") {
		t.Fatalf("unexpected message: %q", apiErr.Message)
	}
}
