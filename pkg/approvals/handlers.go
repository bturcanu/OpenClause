package approvals

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/go-chi/chi/v5"
)

const maxBodyBytes = 1 << 20 // 1 MB

// Handlers groups the HTTP handlers for the approvals service.
type Handlers struct {
	store              handlersStore
	authorizer         *ApproverAuthorizer
	slackSigningSecret string
}

type handlersStore interface {
	CreateRequest(context.Context, CreateApprovalInput) (*ApprovalRequest, error)
	GetRequest(context.Context, string) (*ApprovalRequest, error)
	GrantRequest(context.Context, string, GrantInput) (*ApprovalGrant, error)
	DenyRequest(context.Context, string, DenyInput) error
	ListPending(context.Context, string, int, int) ([]ApprovalRequest, error)
}

// NewHandlers creates handlers backed by the given store.
func NewHandlers(store handlersStore, authorizer *ApproverAuthorizer, slackSigningSecret string) *Handlers {
	return &Handlers{
		store:              store,
		authorizer:         authorizer,
		slackSigningSecret: slackSigningSecret,
	}
}

// RegisterRoutes mounts the approval routes on r.
func (h *Handlers) RegisterRoutes(r chi.Router) {
	r.Post("/v1/approvals/requests", h.CreateRequest)
	r.Get("/v1/approvals/requests/{id}", h.GetRequest)
	r.Post("/v1/approvals/requests/{id}/approve", h.ApproveRequest)
	r.Post("/v1/approvals/requests/{id}/deny", h.DenyRequest)
	r.Get("/v1/approvals/pending", h.ListPending)
}

// CreateRequest handles POST /v1/approvals/requests
func (h *Handlers) CreateRequest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in CreateApprovalInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		types.ErrBadRequest("invalid JSON body").WriteJSON(w)
		return
	}

	if in.TenantID == "" || in.EventID == "" || in.Tool == "" || in.Action == "" {
		types.ErrBadRequest("tenant_id, event_id, tool, and action are required").WriteJSON(w)
		return
	}

	req, err := h.store.CreateRequest(r.Context(), in)
	if err != nil {
		slog.Error("create approval request failed", "error", err)
		types.ErrInternal("failed to create approval request").WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(req); err != nil {
		slog.Error("response encode failed", "error", err)
	}
}

// GetRequest handles GET /v1/approvals/requests/{id}
func (h *Handlers) GetRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, err := h.store.GetRequest(r.Context(), id)
	if err != nil {
		slog.Error("get approval request failed", "error", err)
		types.ErrInternal("failed to retrieve approval request").WriteJSON(w)
		return
	}
	if req == nil {
		types.ErrNotFound("approval request not found").WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(req); err != nil {
		slog.Error("response encode failed", "error", err)
	}
}

// ApproveRequest handles POST /v1/approvals/requests/{id}/approve
func (h *Handlers) ApproveRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in GrantInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		types.ErrBadRequest("invalid JSON body").WriteJSON(w)
		return
	}

	if in.Approver == "" {
		types.ErrBadRequest("approver is required").WriteJSON(w)
		return
	}

	req, err := h.store.GetRequest(r.Context(), id)
	if err != nil {
		slog.Error("get approval request failed", "error", err)
		types.ErrInternal("failed to approve request").WriteJSON(w)
		return
	}
	if req == nil {
		types.ErrNotFound("approval request not found").WriteJSON(w)
		return
	}
	if h.authorizer != nil && !h.authorizer.AllowEmail(req.TenantID, in.Approver) {
		types.ErrForbidden("approver is not allowed for tenant").WriteJSON(w)
		return
	}

	grant, err := h.store.GrantRequest(r.Context(), id, in)
	if err != nil {
		slog.Error("approve request failed", "error", err)
		types.ErrInternal("failed to approve request").WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(grant); err != nil {
		slog.Error("response encode failed", "error", err)
	}
}

// DenyRequest handles POST /v1/approvals/requests/{id}/deny
func (h *Handlers) DenyRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in DenyInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		types.ErrBadRequest("invalid JSON body").WriteJSON(w)
		return
	}

	if in.Approver == "" {
		types.ErrBadRequest("approver is required").WriteJSON(w)
		return
	}

	req, err := h.store.GetRequest(r.Context(), id)
	if err != nil {
		slog.Error("get approval request failed", "error", err)
		types.ErrInternal("failed to deny request").WriteJSON(w)
		return
	}
	if req == nil {
		types.ErrNotFound("approval request not found").WriteJSON(w)
		return
	}
	if h.authorizer != nil && !h.authorizer.AllowEmail(req.TenantID, in.Approver) {
		types.ErrForbidden("approver is not allowed for tenant").WriteJSON(w)
		return
	}

	if err := h.store.DenyRequest(r.Context(), id, in); err != nil {
		slog.Error("deny request failed", "error", err)
		types.ErrInternal("failed to deny request").WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "denied"}); err != nil {
		slog.Error("response encode failed", "error", err)
	}
}

// SlackInteractions handles POST /v1/integrations/slack/interactions.
func (h *Handlers) SlackInteractions(w http.ResponseWriter, r *http.Request) {
	rawBody, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		types.ErrBadRequest("invalid request body").WriteJSON(w)
		return
	}
	if !VerifySlackRequest(rawBody, r.Header.Get("X-Slack-Signature"), r.Header.Get("X-Slack-Request-Timestamp"), h.slackSigningSecret, time.Now()) {
		types.ErrUnauthorized("invalid slack signature").WriteJSON(w)
		return
	}

	form, err := url.ParseQuery(string(rawBody))
	if err != nil {
		types.ErrBadRequest("invalid form body").WriteJSON(w)
		return
	}
	payload := form.Get("payload")
	if payload == "" {
		types.ErrBadRequest("missing payload").WriteJSON(w)
		return
	}

	var in struct {
		Type string `json:"type"`
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"user"`
		Actions []struct {
			Value string `json:"value"`
		} `json:"actions"`
	}
	if err := json.Unmarshal([]byte(payload), &in); err != nil {
		types.ErrBadRequest("invalid interaction payload").WriteJSON(w)
		return
	}
	if in.Type != "block_actions" || len(in.Actions) == 0 {
		types.ErrBadRequest("unsupported interaction type").WriteJSON(w)
		return
	}

	parts := strings.Split(in.Actions[0].Value, "|")
	if len(parts) != 4 {
		types.ErrBadRequest("invalid action value").WriteJSON(w)
		return
	}
	decision, requestID, actionEventID, _ := parts[0], parts[1], parts[2], parts[3]
	req, err := h.store.GetRequest(r.Context(), requestID)
	if err != nil {
		slog.Error("get approval request failed", "error", err, "request_id", requestID)
		types.ErrInternal("failed to process interaction").WriteJSON(w)
		return
	}
	if req == nil {
		types.ErrNotFound("approval request not found").WriteJSON(w)
		return
	}
	if actionEventID != "" && req.EventID != actionEventID {
		types.ErrBadRequest("interaction event mismatch").WriteJSON(w)
		return
	}
	if h.authorizer != nil && !h.authorizer.AllowSlack(req.TenantID, in.User.ID) {
		types.ErrForbidden("slack user is not allowed for tenant").WriteJSON(w)
		return
	}

	approver := "slack:" + in.User.ID
	switch decision {
	case "approve":
		_, err = h.store.GrantRequest(r.Context(), requestID, GrantInput{Approver: approver, MaxUses: 1})
	case "deny":
		err = h.store.DenyRequest(r.Context(), requestID, DenyInput{Approver: approver, Reason: "denied from Slack"})
	default:
		types.ErrBadRequest("unknown action").WriteJSON(w)
		return
	}
	if err != nil {
		slog.Error("slack interaction action failed", "error", err, "request_id", requestID, "decision", decision)
		types.ErrInternal("failed to process interaction").WriteJSON(w)
		return
	}

	username := in.User.Username
	if username == "" {
		username = in.User.Name
	}
	if username == "" {
		username = in.User.ID
	}
	verb := "Processed"
	if decision == "approve" {
		verb = "Approved"
	} else if decision == "deny" {
		verb = "Denied"
	}
	text := fmt.Sprintf("%s by @%s", verb, username)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"text":             text,
		"replace_original": true,
	}); err != nil {
		slog.Error("response encode failed", "error", err)
	}
}

func VerifySlackRequest(rawBody []byte, signatureHeader, timestampHeader, secret string, now time.Time) bool {
	if secret == "" || signatureHeader == "" || timestampHeader == "" {
		return false
	}
	ts, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return false
	}
	reqTime := time.Unix(ts, 0)
	if reqTime.Before(now.Add(-5*time.Minute)) || reqTime.After(now.Add(5*time.Minute)) {
		return false
	}

	base := "v0:" + timestampHeader + ":" + string(rawBody)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(base))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signatureHeader))
}

// ListPending handles GET /v1/approvals/pending?tenant_id=...&limit=...&offset=...
func (h *Handlers) ListPending(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		types.ErrBadRequest("tenant_id query param required").WriteJSON(w)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	reqs, err := h.store.ListPending(r.Context(), tenantID, limit, offset)
	if err != nil {
		slog.Error("list pending failed", "error", err)
		types.ErrInternal("failed to list pending requests").WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(reqs); err != nil {
		slog.Error("response encode failed", "error", err)
	}
}
