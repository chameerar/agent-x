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

// tracer Starts every span. otel.Tracer returns a delegating tracer that is a
// no-op until a real provider is installed, so spans are free when -otel is off.
var tracer = otel.Tracer("agent-x")

// initTracing registers a global TracerProvider that exports spans over OTLP/HTTP
// to a collector (Jaeger). The returned shutdown func flushes batched spans on
// exit — without it the last trace is lost.
func initTracing(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	// WithEndpoint takes host:port (no scheme); WithInsecure means plain HTTP.
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.OTelEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating otlp exporter: %w", err)
	}

	// Resource identifies us on every span, so Jaeger groups traces under "agent-x".
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("agent-x"),
	))
	if err != nil {
		return nil, fmt.Errorf("building resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}
