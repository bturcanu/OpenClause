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

	"github.com/bturcanu/OpenClause/pkg/connectors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	mock := strings.ToLower(os.Getenv("MOCK_CONNECTORS")) == "true"
	token := os.Getenv("SLACK_BOT_TOKEN")

	connector := &SlackConnector{
		log:   log,
		mock:  mock,
		token: token,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(15 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	r.Post("/exec", func(w http.ResponseWriter, r *http.Request) {
		var req connectors.ExecRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}

		resp := connector.Exec(r.Context(), req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	addr := envOr("CONNECTOR_SLACK_ADDR", ":8082")
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		log.Info("connector-slack starting", "addr", addr, "mock", mock)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down connector-slack")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
}

// ──────────────────────────────────────────────────────────────────────────────
// Slack connector implementation
// ──────────────────────────────────────────────────────────────────────────────

type SlackConnector struct {
	log   *slog.Logger
	mock  bool
	token string
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

	// ── Mock mode ────────────────────────────────────────────────────────
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

	// ── Real Slack API ───────────────────────────────────────────────────
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

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: err.Error()}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return connectors.ExecResponse{Status: "error", Error: string(respBody)}
	}

	return connectors.ExecResponse{Status: "success", OutputJSON: respBody}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
