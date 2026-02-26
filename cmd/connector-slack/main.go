// Connector-Slack provides Slack integrations (msg.post) for the gateway.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bturcanu/OpenClause/pkg/config"
	"github.com/bturcanu/OpenClause/pkg/connectors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const maxBodyBytes = 1 << 20 // 1 MB
const maxExternalResponseBytes = 4 << 20

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	mock := strings.ToLower(os.Getenv("MOCK_CONNECTORS")) == "true"
	token := os.Getenv("SLACK_BOT_TOKEN")

	if !mock && token == "" {
		log.Error("SLACK_BOT_TOKEN is required when MOCK_CONNECTORS is not true")
		os.Exit(1)
	}

	connector := &SlackConnector{
		log:   log,
		mock:  mock,
		token: token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}

	internalToken := os.Getenv("INTERNAL_AUTH_TOKEN")

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(15 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	r.Post("/exec", func(w http.ResponseWriter, r *http.Request) {
		if internalToken != "" && r.Header.Get("X-Internal-Token") != internalToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		var req connectors.ExecRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}

		resp := connector.Exec(r.Context(), req)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Error("response encode failed", "error", err)
		}
	})

	addr := config.EnvOr("CONNECTOR_SLACK_ADDR", ":8082")
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("connector-slack starting", "addr", addr, "mock", mock)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down connector-slack")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error("shutdown error", "error", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Slack connector implementation
// ──────────────────────────────────────────────────────────────────────────────

type SlackConnector struct {
	log        *slog.Logger
	mock       bool
	token      string
	httpClient *http.Client
}

type slackMsgParams struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

func (s *SlackConnector) Exec(ctx context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	action := req.Tool + "." + req.Action
	switch action {
	case "slack.msg.post":
		return s.postMessage(ctx, req)
	default:
		return connectors.ExecResponse{
			Status: "error",
			Error:  fmt.Sprintf("unsupported action: %s", action),
		}
	}
}

func (s *SlackConnector) postMessage(ctx context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	var params slackMsgParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "invalid params: " + err.Error()}
	}

	if params.Channel == "" || params.Text == "" {
		return connectors.ExecResponse{Status: "error", Error: "channel and text are required"}
	}

	if s.mock {
		s.log.Info("mock slack.msg.post", "channel", params.Channel, "text_len", len(params.Text))
		output, _ := json.Marshal(map[string]any{
			"ok":      true,
			"channel": params.Channel,
			"ts":      fmt.Sprintf("%d.000000", time.Now().Unix()),
			"mock":    true,
		})
		return connectors.ExecResponse{Status: "success", OutputJSON: output}
	}

	body, _ := json.Marshal(map[string]string{
		"channel": params.Channel,
		"text":    params.Text,
	})

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://slack.com/api/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: err.Error()}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: err.Error()}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxExternalResponseBytes))
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: "read response: " + err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		return connectors.ExecResponse{Status: "error", Error: string(respBody)}
	}

	return connectors.ExecResponse{Status: "success", OutputJSON: respBody}
}
