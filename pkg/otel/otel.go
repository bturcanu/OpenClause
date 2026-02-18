// Package otel provides OpenTelemetry tracing and metrics setup.
package otel

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// Config holds setup parameters.
type Config struct {
	ServiceName    string
	OTLPEndpoint   string // e.g. "localhost:4318"
	MetricsEnabled bool
	TracingEnabled bool
}

// Shutdown is returned by Setup to allow graceful shutdown.
type Shutdown func(ctx context.Context) error

// Setup initializes tracing and metrics exporters.
// Returns a shutdown function that should be deferred.
func Setup(ctx context.Context, cfg Config) (Shutdown, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	var shutdowns []func(ctx context.Context) error

	// ── Tracing ──────────────────────────────────────────────────────────
	if cfg.TracingEnabled && cfg.OTLPEndpoint != "" {
		exporter, err := otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(cfg.OTLPEndpoint),
			otlptracehttp.WithInsecure(),
		)
		if err != nil {
			return nil, fmt.Errorf("otel trace exporter: %w", err)
		}

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
			sdktrace.WithResource(res),
		)
		otel.SetTracerProvider(tp)
		shutdowns = append(shutdowns, tp.Shutdown)
	}

	// ── Propagation ─────────────────────────────────────────────────────
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// ── Metrics (Prometheus) ────────────────────────────────────────────
	if cfg.MetricsEnabled {
		promExporter, err := prometheus.New()
		if err != nil {
			return nil, fmt.Errorf("otel prometheus exporter: %w", err)
		}

		mp := sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(promExporter),
			sdkmetric.WithResource(res),
		)
		otel.SetMeterProvider(mp)
		shutdowns = append(shutdowns, mp.Shutdown)
	}

	shutdown := func(ctx context.Context) error {
		for _, fn := range shutdowns {
			if err := fn(ctx); err != nil {
				slog.Error("otel shutdown", "error", err)
			}
		}
		return nil
	}

	return shutdown, nil
}
