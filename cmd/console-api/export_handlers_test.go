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
	"github.com/bturcanu/OpenClause/pkg/evidence"
	"github.com/bturcanu/OpenClause/pkg/types"
)

type fakeExportStore struct {
	csvErr        error
	bundleErr     error
	csvWriteBytes string
	events        []console.EventDetail
	count         int
	countErr      error
	tenantID      string
	since         time.Time
	until         time.Time
}

func (f *fakeExportStore) ExportEventsCSV(_ context.Context, tenantID string, since, until time.Time, w io.Writer) error {
	f.tenantID = tenantID
	f.since = since
	f.until = until
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

func (f *fakeExportStore) ListEventDetailsInRange(_ context.Context, _ string, since, until time.Time, _ int) ([]console.EventDetail, error) {
	f.since = since
	f.until = until
	if f.bundleErr != nil {
		return nil, f.bundleErr
	}
	return f.events, nil
}

func newTestConsoleAPI(exportStore exportEventsStore) *ConsoleAPI {
	signer, err := evidence.ResolveBundleSigningKey("", "test-export-bundle-secret")
	if err != nil {
		panic(err)
	}
	return &ConsoleAPI{
		log:                  slog.New(slog.NewTextHandler(io.Discard, nil)),
		exportStore:          exportStore,
		evidenceBundleSigner: signer,
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

func TestHandleExportEventsCSV_UsesSinceUntilAndTenantScope(t *testing.T) {
	store := &fakeExportStore{csvWriteBytes: "event_id,tenant_id\nevt-1,tenant-tenant\n"}
	api := newTestConsoleAPI(store)

	claims := &console.JWTClaims{
		Roles:  []string{"tenant_admin"},
		Tenant: "tenant-tenant",
	}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(
		http.MethodGet,
		"/admin/events/export/csv?tenant_id=ignored&since=2020-01-01T00:00:00Z&until=2020-01-02T00:00:00Z",
		nil,
	).WithContext(ctx)

	rr := httptest.NewRecorder()
	api.handleExportEventsCSV(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if store.tenantID != "tenant-tenant" {
		t.Fatalf("expected tenant-scoped export to use claim tenant, got %q", store.tenantID)
	}
	if got := store.since.Format(time.RFC3339); got != "2020-01-01T00:00:00Z" {
		t.Fatalf("expected since to propagate, got %q", got)
	}
	if got := store.until.Format(time.RFC3339); got != "2020-01-02T00:00:00Z" {
		t.Fatalf("expected until to propagate, got %q", got)
	}
	if !strings.Contains(rr.Body.String(), "evt-1,tenant-tenant") {
		t.Fatalf("expected CSV body from export store, got %s", rr.Body.String())
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
		events: []console.EventDetail{{EventListItem: console.EventListItem{EventID: "evt-1"}}},
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

func TestHandleExportBundle_PreservesEventReasonInBundle(t *testing.T) {
	api := newTestConsoleAPI(&fakeExportStore{
		count: 1,
		events: []console.EventDetail{{
			EventListItem: console.EventListItem{
				EventID:  "evt-1",
				TenantID: "tenant-1",
				AgentID:  "agent-1",
				Tool:     "slack",
				Action:   "msg.post",
				Reason:   "export fixture reason",
			},
			PayloadJSON:  json.RawMessage(`{"channel":"C123","text":"hello"}`),
			PolicyResult: json.RawMessage(`{"allow":true,"reason":"export fixture reason"}`),
			Hash:         "hash-1",
			PrevHash:     "hash-0",
			Result: &console.EventResult{
				Status:     "executed",
				OutputJSON: json.RawMessage(`{"ok":true}`),
				DurationMS: 42,
			},
		}},
	})

	claims := &console.JWTClaims{Roles: []string{"platform_admin"}}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(
		http.MethodGet,
		"/admin/reports/export/bundle?tenant_id=tenant-1&since=2020-01-01T00:00:00Z&until=2020-01-02T00:00:00Z",
		nil,
	).WithContext(ctx)

	rr := httptest.NewRecorder()
	api.handleExportBundle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var bundle struct {
		Events            []map[string]any `json:"events"`
		Manifest          map[string]any   `json:"manifest"`
		ManifestSignature string           `json:"manifest_signature"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &bundle); err != nil {
		t.Fatalf("decode export bundle: %v body=%s", err, rr.Body.String())
	}
	if len(bundle.Events) != 1 {
		t.Fatalf("expected one exported event, got %+v", bundle.Events)
	}
	if bundle.Events[0]["reason"] != "export fixture reason" {
		t.Fatalf("expected policy reason in export bundle, got %+v", bundle.Events[0])
	}
	if bundle.Events[0]["payload_json"] == nil || bundle.Events[0]["policy_result"] == nil || bundle.Events[0]["hash"] != "hash-1" || bundle.Events[0]["prev_hash"] != "hash-0" {
		t.Fatalf("expected evidence fields in export bundle, got %+v", bundle.Events[0])
	}
	result, ok := bundle.Events[0]["result"].(map[string]any)
	if !ok || result["status"] != "executed" {
		t.Fatalf("expected execution result in export bundle, got %+v", bundle.Events[0])
	}
	if bundle.ManifestSignature == "" {
		t.Fatalf("expected manifest signature, got %+v", bundle)
	}
	if bundle.Manifest["signature_scheme"] != evidence.SignatureSchemeEd25519 || bundle.Manifest["chain_contiguous"] != true || bundle.Manifest["public_key"] == "" || bundle.Manifest["signing_key_id"] == "" {
		t.Fatalf("expected manifest verification metadata, got %+v", bundle.Manifest)
	}
	manifestCanon, _, err := evidence.HashPayload(bundle.Manifest)
	if err != nil {
		t.Fatalf("HashPayload(manifest): %v", err)
	}
	signer, err := evidence.ResolveBundleSigningKey("", "test-export-bundle-secret")
	if err != nil {
		t.Fatalf("ResolveBundleSigningKey: %v", err)
	}
	expectedSignature := evidence.SignCanonicalPayload(manifestCanon, signer.PrivateKey)
	if bundle.ManifestSignature != expectedSignature {
		t.Fatalf("expected manifest signature %q, got %q", expectedSignature, bundle.ManifestSignature)
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
	details, ok := apiErr.Details.(map[string]any)
	if !ok {
		t.Fatalf("expected details map, got %#v", apiErr.Details)
	}
	if got := details["reason"]; got != "range_too_large" {
		t.Fatalf("expected details.reason range_too_large, got %#v", got)
	}
	if got := details["max_events"]; got != float64(10000) {
		t.Fatalf("expected details.max_events 10000, got %#v", got)
	}
}
