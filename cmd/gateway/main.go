// Gateway is the single entrypoint for AI agent tool-call requests.
// It validates, evaluates policy, routes to connectors, and records evidence.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bturcanu/OpenClause/pkg/approvals"
	"github.com/bturcanu/OpenClause/pkg/auth"
	"github.com/bturcanu/OpenClause/pkg/config"
	"github.com/bturcanu/OpenClause/pkg/connectors"
	"github.com/bturcanu/OpenClause/pkg/evidence"
	ocOtel "github.com/bturcanu/OpenClause/pkg/otel"
	"github.com/bturcanu/OpenClause/pkg/policy"
	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
)

const (
	maxBodyBytes     = 1 << 20 // 1 MB
	maxRateLimiters  = 10_000
	executePollCount = 5
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// ── OpenTelemetry ────────────────────────────────────────────────────
	otelEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	otelShutdown, err := ocOtel.Setup(ctx, ocOtel.Config{
		ServiceName:    config.EnvOr("OTEL_SERVICE_NAME", "oc-gateway"),
		OTLPEndpoint:   otelEndpoint,
		MetricsEnabled: true,
		TracingEnabled: otelEndpoint != "",
	})
	if err != nil {
		log.Error("otel setup failed", "error", err)
	} else {
		defer otelShutdown(context.Background()) //nolint:errcheck // best-effort shutdown
	}

	// ── Postgres ─────────────────────────────────────────────────────────
	pool, err := pgxpool.New(ctx, buildPostgresDSN())
	if err != nil {
		log.Error("postgres connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// ── Dependencies ─────────────────────────────────────────────────────
	evidenceStore := evidence.NewStore(pool)
	evidenceLogger := evidence.NewLogger(evidenceStore, log)
	policyClient := policy.NewClient(config.EnvOr("OPA_URL", "http://localhost:8181"))
	approvalsStore := approvals.NewStore(pool)
	keyStore := auth.NewKeyStore(os.Getenv("API_KEYS"))

	connectorReg := connectors.NewRegistry()
	connectorReg.Register("slack", config.EnvOr("CONNECTOR_SLACK_URL", "http://localhost:8082"))
	connectorReg.Register("jira", config.EnvOr("CONNECTOR_JIRA_URL", "http://localhost:8083"))
	connectorReg.SetInternalToken(os.Getenv("INTERNAL_AUTH_TOKEN"))

	gw := &Gateway{
		log:            log,
		evidence:       evidenceLogger,
		policy:         policyClient,
		connectors:     connectorReg,
		approvals:      approvalsStore,
		approvalsURL:   config.EnvOr("APPROVALS_URL", "http://localhost:8081"),
		rateLimiters:   make(map[string]*rate.Limiter),
		perTenantLimit: config.EnvOrInt("RATE_LIMIT_PER_TENANT", 100),
	}

	// ── Router ───────────────────────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(middleware.Logger)
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
	r.Post("/v1/toolcalls", gw.HandleToolCall)
	r.Get("/v1/toolcalls/{event_id}", gw.HandleGetEvent)
	r.Post("/v1/toolcalls/{event_id}/execute", gw.HandleExecuteToolCall)

	// ── Metrics (internal) ───────────────────────────────────────────────
	metricsAddr := config.EnvOr("METRICS_ADDR", "127.0.0.1:9090")
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsSrv := &http.Server{
		Addr:              metricsAddr,
		Handler:           metricsMux,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	go func() {
		log.Info("metrics server starting", "addr", metricsAddr)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("metrics server error", "error", err)
		}
	}()

	// ── Server ───────────────────────────────────────────────────────────
	addr := config.EnvOr("GATEWAY_ADDR", ":8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("gateway starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down gateway")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error("server shutdown error", "error", err)
	}
	if err := metricsSrv.Shutdown(shutCtx); err != nil {
		log.Error("metrics server shutdown error", "error", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Gateway handler
// ──────────────────────────────────────────────────────────────────────────────

type Gateway struct {
	log            *slog.Logger
	evidence       gatewayEvidence
	policy         gatewayPolicy
	connectors     gatewayConnectors
	approvals      gatewayApprovals
	approvalsURL   string
	rateLimiters   map[string]*rate.Limiter
	rlOrder        []string
	rlMu           sync.Mutex
	perTenantLimit int
}

type gatewayEvidence interface {
	RecordEvent(context.Context, *types.ToolCallEnvelope) error
	CheckIdempotency(context.Context, string, string) (*types.ToolCallResponse, error)
	GetEvent(context.Context, string) (*types.ToolCallEnvelope, error)
	GetExecutionByParentEvent(context.Context, string) (*types.ToolCallResponse, error)
	LinkExecutionToParent(context.Context, string, string, string) (bool, error)
}

type gatewayPolicy interface {
	Evaluate(context.Context, types.PolicyInput) (*types.PolicyResult, error)
}

type gatewayConnectors interface {
	Exec(context.Context, connectors.ExecRequest) (*connectors.ExecResponse, error)
}

type gatewayApprovals interface {
	CreateRequest(context.Context, approvals.CreateApprovalInput) (*approvals.ApprovalRequest, error)
	FindAndConsumeGrant(context.Context, string, string, string, string, string) (*approvals.ApprovalGrant, error)
}

// HandleToolCall is POST /v1/toolcalls
func (gw *Gateway) HandleToolCall(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Parse + validate (with body size limit)
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req types.ToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		types.ErrBadRequest("invalid JSON body").WriteJSON(w)
		return
	}
	if err := req.NormalizeAndValidate(); err != nil {
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
		types.ErrInternal("failed to validate idempotency").WriteJSON(w)
		return
	}
	if prior != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(prior)
		return
	}

	// 4. Build envelope
	eventID := uuid.NewString()
	payloadJSON, err := json.Marshal(req)
	if err != nil {
		gw.log.ErrorContext(ctx, "payload marshal failed", "error", err)
		types.ErrInternal("request processing failed").WriteJSON(w)
		return
	}

	env := &types.ToolCallEnvelope{
		EventID:     eventID,
		Request:     req,
		PayloadJSON: payloadJSON,
		ReceivedAt:  time.Now().UTC(),
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
		if err := gw.evidence.RecordEvent(ctx, env); err != nil {
			gw.log.ErrorContext(ctx, "evidence record failed", "error", err)
		}

	case types.DecisionApprove:
		// Record evidence first so the tool_events row exists before
		// approval_requests references it via FK.
		if err := gw.evidence.RecordEvent(ctx, env); err != nil {
			gw.log.ErrorContext(ctx, "evidence record failed", "error", err)
		}
		approvalReq, err := gw.approvals.CreateRequest(ctx, approvals.CreateApprovalInput{
			EventID:         eventID,
			TenantID:        req.TenantID,
			AgentID:         req.AgentID,
			Tool:            req.Tool,
			Action:          req.Action,
			Resource:        req.Resource,
			RiskScore:       req.RiskScore,
			RiskFactors:     req.RiskFactors,
			Reason:          policyResult.Reason,
			TraceID:         req.TraceID,
			ApproverGroup:   policyResult.ApproverGroup,
			Notify:          policyResult.Notify,
			ApprovalBaseURL: gw.approvalsURL,
		})
		if err != nil {
			gw.log.ErrorContext(ctx, "create approval failed", "error", err)
		} else {
			resp.ApprovalURL = fmt.Sprintf("%s/v1/approvals/requests/%s", gw.approvalsURL, approvalReq.ID)
		}

	case types.DecisionAllow:
		env.ExecutionResult = gw.executeConnector(ctx, eventID, req)
		resp.Result = env.ExecutionResult

		if err := gw.evidence.RecordEvent(ctx, env); err != nil {
			gw.log.ErrorContext(ctx, "evidence record failed", "error", err)
			types.ErrInternal("evidence recording failed after execution").WriteJSON(w)
			return
		}

	default:
		// Fail-closed: treat unrecognized decisions as deny.
		gw.log.ErrorContext(ctx, "unrecognized policy decision, defaulting to deny",
			"decision", string(policyResult.Decision),
			"event_id", eventID,
		)
		env.Decision = types.DecisionDeny
		resp.Decision = types.DecisionDeny
		resp.Reason = "unrecognized policy decision"
		if err := gw.evidence.RecordEvent(ctx, env); err != nil {
			gw.log.ErrorContext(ctx, "evidence record failed", "error", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		gw.log.ErrorContext(ctx, "response encode failed", "error", err)
	}
}

// HandleExecuteToolCall is POST /v1/toolcalls/{event_id}/execute.
// It resumes an approval-gated request once a grant exists and records execution
// as a new append-only evidence event linked to the parent event.
func (gw *Gateway) HandleExecuteToolCall(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	parentEventID := chi.URLParam(r, "event_id")

	if _, err := uuid.Parse(parentEventID); err != nil {
		types.ErrBadRequest("invalid event_id format").WriteJSON(w)
		return
	}

	parent, err := gw.evidence.GetEvent(ctx, parentEventID)
	if err != nil {
		gw.log.ErrorContext(ctx, "get parent event failed", "event_id", parentEventID, "error", err)
		types.ErrInternal("failed to retrieve event").WriteJSON(w)
		return
	}
	if parent == nil {
		types.ErrNotFound("event not found").WriteJSON(w)
		return
	}
	authTenant := auth.TenantFromContext(ctx)
	if authTenant != "" && parent.Request.TenantID != authTenant {
		types.ErrNotFound("event not found").WriteJSON(w)
		return
	}
	if parent.Decision != types.DecisionApprove {
		types.ErrConflict("event does not require approval execution").WriteJSON(w)
		return
	}

	// Idempotent replay by parent event ID.
	existing, err := gw.evidence.GetExecutionByParentEvent(ctx, parentEventID)
	if err != nil {
		gw.log.ErrorContext(ctx, "get linked execution failed", "event_id", parentEventID, "error", err)
		types.ErrInternal("failed to retrieve prior execution").WriteJSON(w)
		return
	}
	if existing != nil {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(existing); err != nil {
			gw.log.ErrorContext(ctx, "response encode failed", "error", err)
		}
		return
	}

	grant, err := gw.approvals.FindAndConsumeGrant(
		ctx,
		parent.Request.TenantID,
		parent.Request.AgentID,
		parent.Request.Tool,
		parent.Request.Action,
		parent.Request.Resource,
	)
	if err != nil {
		gw.log.ErrorContext(ctx, "grant consume failed", "event_id", parentEventID, "error", err)
		types.ErrInternal("failed to consume approval grant").WriteJSON(w)
		return
	}
	if grant == nil {
		// Handle race with an in-flight executor: brief replay polling before
		// returning awaiting-approval.
		for range executePollCount {
			select {
			case <-time.After(50 * time.Millisecond):
			case <-ctx.Done():
				types.ErrInternal("request cancelled").WriteJSON(w)
				return
			}
			existing, err := gw.evidence.GetExecutionByParentEvent(ctx, parentEventID)
			if err != nil {
				gw.log.ErrorContext(ctx, "poll linked execution failed", "event_id", parentEventID, "error", err)
				types.ErrInternal("failed to retrieve prior execution").WriteJSON(w)
				return
			}
			if existing != nil {
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(existing); err != nil {
					gw.log.ErrorContext(ctx, "response encode failed", "error", err)
				}
				return
			}
		}
		types.ErrConflict("awaiting approval").WriteJSON(w)
		return
	}

	execEventID := uuid.NewString()
	payloadJSON, err := json.Marshal(parent.Request)
	if err != nil {
		gw.log.ErrorContext(ctx, "payload marshal failed", "event_id", parentEventID, "error", err)
		types.ErrInternal("request processing failed").WriteJSON(w)
		return
	}

	env := &types.ToolCallEnvelope{
		EventID:     execEventID,
		Request:     parent.Request,
		PayloadJSON: payloadJSON,
		ReceivedAt:  time.Now().UTC(),
		Decision:    types.DecisionAllow,
		PolicyResult: &types.PolicyResult{
			Decision: types.DecisionAllow,
			Reason:   "approved execution",
		},
		ExecutionResult: gw.executeConnector(ctx, execEventID, parent.Request),
	}
	// Avoid conflicting with original request idempotency uniqueness constraint.
	env.Request.IdempotencyKey = "exec:" + parentEventID
	payloadJSON, err = json.Marshal(env.Request)
	if err != nil {
		gw.log.ErrorContext(ctx, "execution payload marshal failed", "event_id", parentEventID, "error", err)
		types.ErrInternal("request processing failed").WriteJSON(w)
		return
	}
	env.PayloadJSON = payloadJSON

	if err := gw.evidence.RecordEvent(ctx, env); err != nil {
		gw.log.ErrorContext(ctx, "execution evidence record failed", "event_id", execEventID, "error", err)
		types.ErrInternal("failed to record execution evidence").WriteJSON(w)
		return
	}

	linked, err := gw.evidence.LinkExecutionToParent(ctx, parentEventID, execEventID, grant.ID)
	if err != nil {
		gw.log.ErrorContext(ctx, "link execution failed", "parent_event_id", parentEventID, "execution_event_id", execEventID, "error", err)
		types.ErrInternal("failed to finalize execution").WriteJSON(w)
		return
	}
	if !linked {
		// Another concurrent request linked first; return canonical replay response.
		prior, err := gw.evidence.GetExecutionByParentEvent(ctx, parentEventID)
		if err != nil {
			gw.log.ErrorContext(ctx, "get concurrent linked execution failed", "event_id", parentEventID, "error", err)
			types.ErrInternal("failed to retrieve prior execution").WriteJSON(w)
			return
		}
		if prior != nil {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(prior); err != nil {
				gw.log.ErrorContext(ctx, "response encode failed", "error", err)
			}
			return
		}
	}

	resp := types.ToolCallResponse{
		EventID:  execEventID,
		Decision: types.DecisionAllow,
		Reason:   "approved execution",
		Result:   env.ExecutionResult,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		gw.log.ErrorContext(ctx, "response encode failed", "error", err)
	}
}

// HandleGetEvent is GET /v1/toolcalls/{event_id}
func (gw *Gateway) HandleGetEvent(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "event_id")

	if _, err := uuid.Parse(eventID); err != nil {
		types.ErrBadRequest("invalid event_id format").WriteJSON(w)
		return
	}

	env, err := gw.evidence.GetEvent(r.Context(), eventID)
	if err != nil {
		gw.log.ErrorContext(r.Context(), "get event failed", "error", err)
		types.ErrInternal("failed to retrieve event").WriteJSON(w)
		return
	}
	if env == nil {
		types.ErrNotFound("event not found").WriteJSON(w)
		return
	}
	authTenant := auth.TenantFromContext(r.Context())
	if authTenant != "" && env.Request.TenantID != authTenant {
		types.ErrNotFound("event not found").WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(env); err != nil {
		gw.log.ErrorContext(r.Context(), "response encode failed", "error", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Rate limiting (bounded map with eviction)
// ──────────────────────────────────────────────────────────────────────────────

func (gw *Gateway) allowRate(tenantID string) bool {
	gw.rlMu.Lock()
	defer gw.rlMu.Unlock()

	lim, ok := gw.rateLimiters[tenantID]
	if ok {
		// Move to end of LRU order.
		for i, k := range gw.rlOrder {
			if k == tenantID {
				gw.rlOrder = append(gw.rlOrder[:i], gw.rlOrder[i+1:]...)
				break
			}
		}
		gw.rlOrder = append(gw.rlOrder, tenantID)
		return lim.Allow()
	}

	if len(gw.rateLimiters) >= maxRateLimiters {
		oldest := gw.rlOrder[0]
		gw.rlOrder = gw.rlOrder[1:]
		delete(gw.rateLimiters, oldest)
	}

	lim = rate.NewLimiter(rate.Limit(gw.perTenantLimit), gw.perTenantLimit*2)
	gw.rateLimiters[tenantID] = lim
	gw.rlOrder = append(gw.rlOrder, tenantID)
	return lim.Allow()
}

func (gw *Gateway) executeConnector(ctx context.Context, eventID string, req types.ToolCallRequest) *types.ExecutionResult {
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
		return &types.ExecutionResult{
			Status:     "error",
			Error:      err.Error(),
			DurationMS: duration.Milliseconds(),
		}
	}
	return &types.ExecutionResult{
		Status:     execResp.Status,
		OutputJSON: execResp.OutputJSON,
		Error:      execResp.Error,
		DurationMS: duration.Milliseconds(),
	}
}

func buildPostgresDSN() string {
	sslmode := config.EnvOr("POSTGRES_SSLMODE", "disable")
	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(config.EnvOr("POSTGRES_USER", "openclause"), config.EnvOr("POSTGRES_PASSWORD", "changeme")),
		Host:     net.JoinHostPort(config.EnvOr("POSTGRES_HOST", "localhost"), config.EnvOr("POSTGRES_PORT", "5432")),
		Path:     config.EnvOr("POSTGRES_DB", "openclause"),
		RawQuery: "sslmode=" + url.QueryEscape(sslmode),
	}
	return u.String()
}
