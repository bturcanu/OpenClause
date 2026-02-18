package approvals

import (
	"encoding/json"
	"net/http"

	"github.com/agenticaccess/governance/pkg/types"
	"github.com/go-chi/chi/v5"
)

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
	var in CreateApprovalInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		types.ErrBadRequest("invalid JSON body").WriteJSON(w)
		return
	}

	req, err := h.store.CreateRequest(r.Context(), in)
	if err != nil {
		types.ErrInternal(err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(req)
}

// GetRequest handles GET /v1/approvals/requests/{id}
func (h *Handlers) GetRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, err := h.store.GetRequest(r.Context(), id)
	if err != nil {
		types.ErrInternal(err.Error()).WriteJSON(w)
		return
	}
	if req == nil {
		types.ErrNotFound("approval request not found").WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(req)
}

// ApproveRequest handles POST /v1/approvals/requests/{id}/approve
func (h *Handlers) ApproveRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in GrantInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		types.ErrBadRequest("invalid JSON body").WriteJSON(w)
		return
	}

	grant, err := h.store.GrantRequest(r.Context(), id, in)
	if err != nil {
		types.ErrInternal(err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(grant)
}

// DenyRequest handles POST /v1/approvals/requests/{id}/deny
func (h *Handlers) DenyRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in DenyInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		types.ErrBadRequest("invalid JSON body").WriteJSON(w)
		return
	}

	if err := h.store.DenyRequest(r.Context(), id, in); err != nil {
		types.ErrInternal(err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "denied"})
}

// ListPending handles GET /v1/approvals/pending?tenant_id=...
func (h *Handlers) ListPending(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		types.ErrBadRequest("tenant_id query param required").WriteJSON(w)
		return
	}

	reqs, err := h.store.ListPending(r.Context(), tenantID)
	if err != nil {
		types.ErrInternal(err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reqs)
}
