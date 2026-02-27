package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/bturcanu/OpenClause/pkg/config"
	"github.com/bturcanu/OpenClause/pkg/connectors"
	"github.com/bturcanu/OpenClause/pkg/connectors/sdk"
)

type templateConnector struct{}

func (t templateConnector) Exec(_ context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	output, _ := json.Marshal(map[string]any{
		"tool":     req.Tool,
		"action":   req.Action,
		"resource": req.Resource,
		"mock":     true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: output}
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	addr := config.EnvOr("CONNECTOR_TEMPLATE_ADDR", ":8099")
	internalToken := os.Getenv("INTERNAL_AUTH_TOKEN")

	mux := http.NewServeMux()
	mux.HandleFunc("/exec", sdk.Handler(templateConnector{}, sdk.Config{
		InternalToken: internalToken,
		Logger:        log,
	}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	log.Info("connector-template starting", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil && err != http.ErrServerClosed {
		log.Error("server error", "error", err)
		os.Exit(1)
	}
}
