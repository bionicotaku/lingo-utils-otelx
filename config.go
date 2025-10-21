package otelx

import (
	"fmt"
	"strings"
)

// ExporterType enumerates supported OpenTelemetry exporters.
type ExporterType string

const (
	ExporterStdout     ExporterType = "stdout"
	ExporterOTLP       ExporterType = "otlp"
	ExporterCloudTrace ExporterType = "cloudtrace"
)

// DefaultSamplingRatio defines the fallback trace sampling ratio when none is provided.
const DefaultSamplingRatio = 0.1

// Config controls how otelx initializes tracing.
type Config struct {
	ServiceName    string `json:"serviceName"`
	ServiceVersion string `json:"serviceVersion"`
	Environment    string `json:"environment"`

	Exporter      ExporterType      `json:"exporter"`
	SamplingRatio *float64          `json:"samplingRatio"`
	Endpoint      string            `json:"endpoint"`
	Insecure      bool              `json:"insecure"`
	GCPProjectID  string            `json:"gcpProjectId"`
	Headers       map[string]string `json:"headers"`
	ResourceAttrs map[string]string `json:"resourceAttrs"`
}

// sanitize trims spaces from string fields and normalises exporter value.
func (cfg Config) sanitize() Config {
	cfg.ServiceName = strings.TrimSpace(cfg.ServiceName)
	cfg.ServiceVersion = strings.TrimSpace(cfg.ServiceVersion)
	cfg.Environment = strings.TrimSpace(cfg.Environment)
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.GCPProjectID = strings.TrimSpace(cfg.GCPProjectID)
	cfg.Exporter = ExporterType(strings.ToLower(string(cfg.Exporter)))
	return cfg
}

// validate performs semantic validation of the config.
func (cfg Config) validate() error {
	if cfg.ServiceName == "" {
		return fmt.Errorf("otelx: serviceName is required")
	}

	switch cfg.Exporter {
	case "", ExporterStdout, ExporterOTLP, ExporterCloudTrace:
		// ok
	default:
		return fmt.Errorf("otelx: unsupported exporter %q", cfg.Exporter)
	}

	if cfg.SamplingRatio != nil {
		if ratio := *cfg.SamplingRatio; ratio < 0 || ratio > 1 {
			return fmt.Errorf("otelx: samplingRatio must be within [0,1], got %v", ratio)
		}
	}

	if cfg.Exporter == ExporterCloudTrace && cfg.GCPProjectID == "" {
		return fmt.Errorf("otelx: gcpProjectId is required when exporter=cloudtrace")
	}

	return nil
}

// Float64 is a helper that returns a pointer to the provided float64.
func Float64(v float64) *float64 {
	return &v
}
