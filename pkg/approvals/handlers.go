package approvals

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/go-chi/chi/v5"
)

const maxBodyBytes = 1 << 20 // 1 MB

// Handlers groups the HTTP handlers for the approvals service.
type Handlers struct {
	store *Store
}

// NewHandlers creates handlers backed by the given store.
func NewHandlers(store *Store) *Handlers {
	return &Handlers{store: store}
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
