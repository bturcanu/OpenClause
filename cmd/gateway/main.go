// Gateway is the single entrypoint for AI agent tool-call requests.
// It validates, evaluates policy, routes to connectors, and records evidence.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/agenticaccess/governance/pkg/approvals"
	"github.com/agenticaccess/governance/pkg/auth"
	"github.com/agenticaccess/governance/pkg/connectors"
	"github.com/agenticaccess/governance/pkg/evidence"
	aagOtel "github.com/agenticaccess/governance/pkg/otel"
	"github.com/agenticaccess/governance/pkg/policy"
	"github.com/agenticaccess/governance/pkg/types"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// ── OpenTelemetry ────────────────────────────────────────────────────
	otelShutdown, err := aagOtel.Setup(ctx, aagOtel.Config{
		ServiceName:    envOr("OTEL_SERVICE_NAME", "aag-gateway"),
		OTLPEndpoint:   os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		MetricsEnabled: true,
		TracingEnabled: os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "",
	})
	if err != nil {
		log.Error("otel setup failed", "error", err)
	} else {
		defer otelShutdown(context.Background())
	}

	// ── Postgres ─────────────────────────────────────────────────────────
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		envOr("POSTGRES_USER", "aag"),
		envOr("POSTGRES_PASSWORD", "changeme"),
		envOr("POSTGRES_HOST", "localhost"),
		envOr("POSTGRES_PORT", "5432"),
		envOr("POSTGRES_DB", "aag"),
	)
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Error("postgres connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// ── Dependencies ─────────────────────────────────────────────────────
	evidenceStore := evidence.NewStore(pool)
	evidenceLogger := evidence.NewLogger(evidenceStore, log)
	policyClient := policy.NewClient(envOr("OPA_URL", "http://localhost:8181"))
	approvalsStore := approvals.NewStore(pool)
	keyStore := auth.NewKeyStore(os.Getenv("API_KEYS"))

	connectorReg := connectors.NewRegistry()
	connectorReg.Register("slack", envOr("CONNECTOR_SLACK_URL", "http://localhost:8082"))
	connectorReg.Register("jira", envOr("CONNECTOR_JIRA_URL", "http://localhost:8083"))

	gw := &Gateway{
		log:            log,
		evidence:       evidenceLogger,
		policy:         policyClient,
		connectors:     connectorReg,
		approvals:      approvalsStore,
		approvalsURL:   envOr("APPROVALS_URL", "http://localhost:8081"),
		rateLimiters:   make(map[string]*rate.Limiter),
		perTenantLimit: envOrInt("RATE_LIMIT_PER_TENANT", 100),
	}

	// ── Router ───────────────────────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(auth.APIKeyAuth(keyStore))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("NOT READY"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	r.Handle("/metrics", promhttp.Handler())

	r.Post("/v1/toolcalls", gw.HandleToolCall)
	r.Get("/v1/toolcalls/{event_id}", gw.HandleGetEvent)

	// ── Server ───────────────────────────────────────────────────────────
	addr := envOr("GATEWAY_ADDR", ":8080")
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		log.Info("gateway starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down gateway")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
}

// ──────────────────────────────────────────────────────────────────────────────
// Gateway handler
// ──────────────────────────────────────────────────────────────────────────────

type Gateway struct {
	log            *slog.Logger
	evidence       *evidence.Logger
	policy         *policy.Client
	connectors     *connectors.Registry
	approvals      *approvals.Store
	approvalsURL   string
	rateLimiters   map[string]*rate.Limiter
	rlMu           sync.Mutex
	perTenantLimit int
}

// HandleToolCall is POST /v1/toolcalls
func (gw *Gateway) HandleToolCall(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Parse + validate
	var req types.ToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		types.ErrBadRequest("invalid JSON body").WriteJSON(w)
		return
	}
	if err := req.Validate(); err != nil {
		types.ErrValidation(err).WriteJSON(w)
		return
	}

	// Override tenant from auth context
	if t := auth.TenantFromContext(ctx); t != "" {
		req.TenantID = t
	}

	// 2. Rate limit
	if !gw.allowRate(req.TenantID) {
		types.ErrRateLimited().WriteJSON(w)
		return
	}

	// 3. Idempotency
	prior, err := gw.evidence.CheckIdempotency(ctx, req.TenantID, req.IdempotencyKey)
	if err != nil {
		gw.log.ErrorContext(ctx, "idempotency check failed", "error", err)
		// Continue (non-fatal), but log it
	}
	if prior != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(prior)
		return
	}

	// 4. Build envelope
	eventID := uuid.NewString()
	payloadJSON, _ := json.Marshal(req)

	env := &types.ToolCallEnvelope{
		EventID:    eventID,
		Request:    req,
		PayloadJSON: payloadJSON,
		ReceivedAt: time.Now().UTC(),
	}

	// 5. Evaluate policy
	policyInput := types.PolicyInput{
		ToolCall: req,
		Environment: types.PolicyEnvironment{
			Timestamp: time.Now().UTC(),
		},
	}

	policyResult, err := gw.policy.Evaluate(ctx, policyInput)
	if err != nil {
		gw.log.ErrorContext(ctx, "policy evaluation failed", "error", err)
		// Fail closed — deny
		policyResult = &types.PolicyResult{Decision: types.DecisionDeny, Reason: "policy evaluation failed"}
	}
	env.Decision = policyResult.Decision
	env.PolicyResult = policyResult

	// 6. Act on decision
	resp := types.ToolCallResponse{
		EventID:  eventID,
		Decision: policyResult.Decision,
		Reason:   policyResult.Reason,
	}

	switch policyResult.Decision {
	case types.DecisionDeny:
		// Just record and return
		if err := gw.evidence.RecordEvent(ctx, env); err != nil {
			gw.log.ErrorContext(ctx, "evidence record failed", "error", err)
		}

	case types.DecisionApprove:
		// Create an approval request
		approvalReq, err := gw.approvals.CreateRequest(ctx, approvals.CreateApprovalInput{
			EventID:   eventID,
			TenantID:  req.TenantID,
			AgentID:   req.AgentID,
			Tool:      req.Tool,
			Action:    req.Action,
			Resource:  req.Resource,
			RiskScore: req.RiskScore,
			Reason:    policyResult.Reason,
		})
		if err != nil {
			gw.log.ErrorContext(ctx, "create approval failed", "error", err)
		} else {
			resp.ApprovalURL = fmt.Sprintf("%s/v1/approvals/requests/%s", gw.approvalsURL, approvalReq.ID)
		}
		if err := gw.evidence.RecordEvent(ctx, env); err != nil {
			gw.log.ErrorContext(ctx, "evidence record failed", "error", err)
		}

	case types.DecisionAllow:
		// Execute the connector
		start := time.Now()
		execResp, err := gw.connectors.Exec(ctx, connectors.ExecRequest{
			EventID:  eventID,
			TenantID: req.TenantID,
			AgentID:  req.AgentID,
			Tool:     req.Tool,
			Action:   req.Action,
			Params:   req.Params,
			Resource: req.Resource,
		})
		duration := time.Since(start)

		if err != nil {
			env.ExecutionResult = &types.ExecutionResult{
				Status:     "error",
				Error:      err.Error(),
				DurationMS: duration.Milliseconds(),
			}
		} else {
			env.ExecutionResult = &types.ExecutionResult{
				Status:     execResp.Status,
				OutputJSON: execResp.OutputJSON,
				Error:      execResp.Error,
				DurationMS: duration.Milliseconds(),
			}
		}
		resp.Result = env.ExecutionResult

		if err := gw.evidence.RecordEvent(ctx, env); err != nil {
			gw.log.ErrorContext(ctx, "evidence record failed", "error", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleGetEvent is GET /v1/toolcalls/{event_id}
func (gw *Gateway) HandleGetEvent(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "event_id")
	env, err := gw.evidence.GetEvent(r.Context(), eventID)
	if err != nil {
		types.ErrInternal(err.Error()).WriteJSON(w)
		return
	}
	if env == nil {
		types.ErrNotFound("event not found").WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(env)
}

// ──────────────────────────────────────────────────────────────────────────────
// Rate limiting
// ──────────────────────────────────────────────────────────────────────────────

func (gw *Gateway) allowRate(tenantID string) bool {
	gw.rlMu.Lock()
	defer gw.rlMu.Unlock()

	lim, ok := gw.rateLimiters[tenantID]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(gw.perTenantLimit), gw.perTenantLimit*2)
		gw.rateLimiters[tenantID] = lim
	}
	return lim.Allow()
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
