package main

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// tracer is the handle we Start spans from. otel.Tracer returns a delegating
// tracer: until a real TracerProvider is installed it is a no-op, so every
// span in the codebase is free when -otel is off. initTracing swaps in the
// real provider at startup.
var tracer = otel.Tracer("agent-x")

// initTracing builds a TracerProvider that exports spans over OTLP/HTTP to a
// collector (Jaeger) and registers it globally. It returns a shutdown func
// that flushes buffered spans — spans are batched, so without this final
// flush the last trace is lost on exit.
func initTracing(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	// Exporter: encodes finished spans as OTLP and POSTs them to the collector.
	// WithEndpoint takes host:port (no scheme); WithInsecure means plain HTTP,
	// which is correct for a local Jaeger.
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.OTelEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating otlp exporter: %w", err)
	}

	// Resource: identity attached to every span, so Jaeger files all our traces
	// under the "agent-x" service.
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("agent-x"),
	))
	if err != nil {
		return nil, fmt.Errorf("building resource: %w", err)
	}

	// Provider: owns the exporter + resource, batches spans in the background.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}
