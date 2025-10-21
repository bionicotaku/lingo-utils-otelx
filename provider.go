package otelx

import (
	"context"
	"fmt"
	"strings"
	"time"

	logx "github.com/bionicotaku/lingo-utils-logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// Provider bundles the TracerProvider, Propagator and shutdown hook created by Setup.
type Provider struct {
	TP         *sdktrace.TracerProvider
	Propagator propagation.TextMapPropagator
	shutdown   func(context.Context) error
}

// Shutdown flushes remaining spans and releases exporter resources.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || p.shutdown == nil {
		return nil
	}
	return p.shutdown(ctx)
}

// Setup initialises OpenTelemetry tracing according to Config.
func Setup(ctx context.Context, cfg Config, logger logx.Logger, opts ...Option) (*Provider, error) {
	cfg = cfg.sanitize()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	options := &setupOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}

	exporter, err := buildExporter(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	sampler := DefaultSamplingRatio
	if cfg.SamplingRatio != nil {
		sampler = *cfg.SamplingRatio
	}
	if options.samplerHook != nil {
		options.samplerHook(sampler)
	}

	resourceOpts := []resource.Option{
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithHost(),
		resource.WithTelemetrySDK(),
	}

	attrs := []attribute.KeyValue{semconv.ServiceName(cfg.ServiceName)}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}
	if cfg.Environment != "" {
		attrs = append(attrs, semconv.DeploymentEnvironmentName(cfg.Environment))
	}
	for k, v := range cfg.ResourceAttrs {
		if strings.TrimSpace(k) == "" {
			continue
		}
		attrs = append(attrs, attribute.String(k, v))
	}
	resourceOpts = append(resourceOpts, resource.WithAttributes(attrs...))
	if len(options.resourceOpts) > 0 {
		resourceOpts = append(resourceOpts, options.resourceOpts...)
	}

	res, err := resource.New(ctx, resourceOpts...)
	if err != nil {
		return nil, fmt.Errorf("otelx: build resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampler))),
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
	)

	prop := options.propagator
	if prop == nil {
		prop = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	}

	if options.global {
		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(prop)
	}

	return &Provider{
		TP:         tp,
		Propagator: prop,
		shutdown: func(ctx context.Context) error {
			return tp.Shutdown(ctx)
		},
	}, nil
}
