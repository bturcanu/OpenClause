package sdk

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/bturcanu/OpenClause/pkg/connectors"
)

const maxBodyBytes = 1 << 20

type Executor interface {
	Exec(context.Context, connectors.ExecRequest) connectors.ExecResponse
}

type Config struct {
	InternalToken string
	Logger        *slog.Logger
}

func Handler(executor Executor, cfg Config) http.HandlerFunc {
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.InternalToken != "" && r.Header.Get("X-Internal-Token") != cfg.InternalToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		var req connectors.ExecRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		resp := executor.Exec(ctx, req)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Error("encode response failed", "error", err)
		}
	}
}
