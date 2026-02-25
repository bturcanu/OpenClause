// Approvals service manages approval requests and grants for tool-call governance.
package main

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bturcanu/OpenClause/pkg/approvals"
	ocOtel "github.com/bturcanu/OpenClause/pkg/otel"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// ── OpenTelemetry ────────────────────────────────────────────────────
	otelShutdown, err := ocOtel.Setup(ctx, ocOtel.Config{
		ServiceName:    "oc-approvals",
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
		envOr("POSTGRES_USER", "openclause"),
		envOr("POSTGRES_PASSWORD", "changeme"),
		envOr("POSTGRES_HOST", "localhost"),
		envOr("POSTGRES_PORT", "5432"),
		envOr("POSTGRES_DB", "openclause"),
	)
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Error("postgres connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := approvals.NewStore(pool)
	handlers := approvals.NewHandlers(store)

	// ── Router ───────────────────────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(15 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Register API routes
	handlers.RegisterRoutes(r)

	// ── Minimal web UI for pending approvals ─────────────────────────────
	r.Get("/ui/pending", func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.URL.Query().Get("tenant_id")
		if tenantID == "" {
			http.Error(w, "tenant_id required", http.StatusBadRequest)
			return
		}
		reqs, err := store.ListPending(r.Context(), tenantID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pendingTmpl.Execute(w, struct {
			TenantID string
			Requests []approvals.ApprovalRequest
		}{TenantID: tenantID, Requests: reqs})
	})

	// ── Server ───────────────────────────────────────────────────────────
	addr := envOr("APPROVALS_ADDR", ":8081")
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		log.Info("approvals service starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down approvals service")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ──────────────────────────────────────────────────────────────────────────────
// Minimal server-rendered UI
// ──────────────────────────────────────────────────────────────────────────────

var pendingTmpl = template.Must(template.New("pending").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Pending Approvals — {{.TenantID}}</title>
  <style>
    body { font-family: system-ui, sans-serif; max-width: 900px; margin: 2rem auto; padding: 0 1rem; }
    table { width: 100%; border-collapse: collapse; margin-top: 1rem; }
    th, td { text-align: left; padding: 0.5rem 0.75rem; border-bottom: 1px solid #e2e8f0; }
    th { background: #f7fafc; font-weight: 600; }
    tr:hover { background: #edf2f7; }
    .badge { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 0.85em; }
    .badge-pending { background: #fefcbf; color: #744210; }
    .risk-high { color: #c53030; font-weight: 600; }
    h1 { color: #2d3748; }
    .empty { color: #718096; padding: 2rem 0; }
  </style>
</head>
<body>
  <h1>Pending Approvals</h1>
  <p>Tenant: <strong>{{.TenantID}}</strong></p>
  {{if .Requests}}
  <table>
    <thead>
      <tr><th>ID</th><th>Tool</th><th>Action</th><th>Agent</th><th>Risk</th><th>Reason</th><th>Created</th></tr>
    </thead>
    <tbody>
      {{range .Requests}}
      <tr>
        <td><code>{{.ID}}</code></td>
        <td>{{.Tool}}</td>
        <td>{{.Action}}</td>
        <td>{{.AgentID}}</td>
        <td {{if ge .RiskScore 7}}class="risk-high"{{end}}>{{.RiskScore}}</td>
        <td>{{.Reason}}</td>
        <td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
  {{else}}
  <p class="empty">No pending approvals.</p>
  {{end}}
</body>
</html>`))
