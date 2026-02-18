package types

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ──────────────────────────────────────────────────────────────────────────────
// Validation error (returned during request parsing)
// ──────────────────────────────────────────────────────────────────────────────

type ValidationError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation: %s %s", e.Field, e.Reason)
}

// ──────────────────────────────────────────────────────────────────────────────
// APIError — structured error returned to callers
// ──────────────────────────────────────────────────────────────────────────────

type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Details   any    `json:"details,omitempty"`
	HTTPCode  int    `json:"-"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// WriteJSON writes the error as JSON to the response writer.
func (e *APIError) WriteJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPCode)
	_ = json.NewEncoder(w).Encode(e)
}

// ──────────────────────────────────────────────────────────────────────────────
// Common error constructors
// ──────────────────────────────────────────────────────────────────────────────

func ErrBadRequest(msg string) *APIError {
	return &APIError{Code: "BAD_REQUEST", Message: msg, HTTPCode: http.StatusBadRequest}
}

func ErrValidation(err error) *APIError {
	return &APIError{Code: "VALIDATION_ERROR", Message: err.Error(), HTTPCode: http.StatusUnprocessableEntity}
}

func ErrUnauthorized(msg string) *APIError {
	return &APIError{Code: "UNAUTHORIZED", Message: msg, HTTPCode: http.StatusUnauthorized}
}

func ErrForbidden(msg string) *APIError {
	return &APIError{Code: "FORBIDDEN", Message: msg, HTTPCode: http.StatusForbidden}
}

func ErrNotFound(msg string) *APIError {
	return &APIError{Code: "NOT_FOUND", Message: msg, HTTPCode: http.StatusNotFound}
}

func ErrConflict(msg string) *APIError {
	return &APIError{Code: "CONFLICT", Message: msg, HTTPCode: http.StatusConflict}
}

func ErrInternal(msg string) *APIError {
	return &APIError{Code: "INTERNAL_ERROR", Message: msg, Retryable: true, HTTPCode: http.StatusInternalServerError}
}

func ErrRateLimited() *APIError {
	return &APIError{Code: "RATE_LIMITED", Message: "too many requests", Retryable: true, HTTPCode: http.StatusTooManyRequests}
}

func ErrConnectorTimeout(tool string) *APIError {
	return &APIError{Code: "CONNECTOR_TIMEOUT", Message: fmt.Sprintf("connector %s timed out", tool), Retryable: true, HTTPCode: http.StatusGatewayTimeout}
}

func ErrConnectorFailure(tool, detail string) *APIError {
	return &APIError{Code: "CONNECTOR_ERROR", Message: fmt.Sprintf("connector %s failed: %s", tool, detail), Retryable: false, HTTPCode: http.StatusBadGateway}
}

func ErrPolicyDenied(reason string) *APIError {
	return &APIError{Code: "POLICY_DENIED", Message: reason, HTTPCode: http.StatusForbidden}
}
