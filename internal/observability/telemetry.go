package observability

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// TelemetryOptions configures OpenTelemetry setup.
type TelemetryOptions struct {
	ServiceName    string
	ServiceVersion string
	Environment    string

	// OTLPEndpoint is the collector base URL. When empty, no exporters are
	// installed and Setup is a no-op returning a no-op shutdown.
	OTLPEndpoint string
}

// Shutdown flushes and releases telemetry providers. It is always non-nil, so
// callers can defer it unconditionally.
type Shutdown func(context.Context) error

// Setup installs global tracer and meter providers plus the W3C propagators.
//
// The global providers default to no-ops, so instrumentation code can call
// otel.Tracer(...) and otel.Meter(...) unconditionally whether or not export is
// enabled. Only the exporters are conditional.
func Setup(ctx context.Context, opts TelemetryOptions) (Shutdown, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if opts.OTLPEndpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(opts.ServiceName),
		semconv.ServiceVersion(opts.ServiceVersion),
		semconv.DeploymentEnvironmentNameKey.String(opts.Environment),
	))
	if err != nil {
		return nil, fmt.Errorf("build otel resource: %w", err)
	}

	traceExp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(opts.OTLPEndpoint))
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)

	metricExp, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(opts.OTLPEndpoint))
	if err != nil {
		// Tear down the trace provider we already installed rather than
		// leaking its background batcher goroutine.
		_ = tracerProvider.Shutdown(ctx)
		return nil, fmt.Errorf("create otlp metric exporter: %w", err)
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	return func(ctx context.Context) error {
		return errors.Join(
			tracerProvider.Shutdown(ctx),
			meterProvider.Shutdown(ctx),
		)
	}, nil
}
