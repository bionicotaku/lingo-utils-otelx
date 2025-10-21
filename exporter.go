package otelx

import (
	"context"
	"fmt"
	"time"

	cloudtrace "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	logx "github.com/bionicotaku/lingo-utils-logx"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func buildExporter(ctx context.Context, cfg Config, logger logx.Logger) (sdktrace.SpanExporter, error) {
	logCtx := ctx

	switch cfg.Exporter {
	case "", ExporterStdout:
		exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("otelx: create stdout exporter: %w", err)
		}
		if logger != nil {
			logger.Debug(logCtx, "otelx.exporter.stdout.enabled")
		}
		return exporter, nil

	case ExporterOTLP:
		options := []otlptracegrpc.Option{}
		if cfg.Endpoint != "" {
			options = append(options, otlptracegrpc.WithEndpoint(cfg.Endpoint))
		}
		if cfg.Insecure {
			options = append(options, otlptracegrpc.WithInsecure())
		}
		if len(cfg.Headers) > 0 {
			options = append(options, otlptracegrpc.WithHeaders(cfg.Headers))
		}

		exporter, err := otlptracegrpc.New(ctx, options...)
		if err != nil {
			return nil, fmt.Errorf("otelx: create otlp exporter: %w", err)
		}
		if logger != nil {
			logger.Info(logCtx, "otelx.exporter.otlp.enabled")
		}
		return exporter, nil

	case ExporterCloudTrace:
		exporter, err := cloudtrace.New(
			cloudtrace.WithProjectID(cfg.GCPProjectID),
			cloudtrace.WithContext(ctx),
			cloudtrace.WithTimeout(10*time.Second),
		)
		if err != nil {
			return nil, fmt.Errorf("otelx: create cloudtrace exporter: %w", err)
		}
		if logger != nil {
			logger.Info(logCtx, "otelx.exporter.cloudtrace.enabled")
		}
		return exporter, nil

	default:
		return nil, fmt.Errorf("otelx: unsupported exporter %q", cfg.Exporter)
	}
}
