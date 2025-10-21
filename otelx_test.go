package otelx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	logx "github.com/bionicotaku/lingo-utils-logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func TestSetupRequiresServiceName(t *testing.T) {
	_, err := Setup(context.Background(), Config{}, nil)
	if err == nil {
		t.Fatalf("expected error for missing service name")
	}
	if !strings.Contains(err.Error(), "serviceName") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestSetupStdoutDefault(t *testing.T) {
	prov, err := Setup(context.Background(), Config{ServiceName: "svc"}, noopLogger{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if prov == nil || prov.TP == nil {
		t.Fatalf("expected provider")
	}
	if prov.Propagator == nil {
		t.Fatalf("expected propagator")
	}
	if err := prov.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}

func TestSetupWithGlobal(t *testing.T) {
	restore := saveGlobal()
	defer restore()

	prop := propagation.NewCompositeTextMapPropagator(propagation.Baggage{})
	prov, err := Setup(context.Background(), Config{ServiceName: "svc"}, nil, WithGlobal(), WithPropagator(prop))
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if got := otel.GetTextMapPropagator(); !reflect.DeepEqual(got, prop) {
		t.Fatalf("expected global propagator to be set")
	}
	if err := prov.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}

func TestSetupWithSamplingRatio(t *testing.T) {
	prov, err := Setup(context.Background(), Config{ServiceName: "svc", SamplingRatio: 0.05}, nil)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	_ = prov.Shutdown(context.Background())
}

func TestSetupOTLPExporter(t *testing.T) {
	tctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	cfg := Config{ServiceName: "svc", Exporter: ExporterOTLP, Endpoint: "localhost:4317", Insecure: true}
	prov, err := Setup(tctx, cfg, noopLogger{})
	if err != nil {
		// Depending on environment, OTLP exporter may attempt connection eagerly. Allow error containing hint.
		if !strings.Contains(err.Error(), "otlp exporter") {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	_ = prov.Shutdown(context.Background())
}

func TestSetupCloudTraceExporterValidation(t *testing.T) {
	cfg := Config{ServiceName: "svc", Exporter: ExporterCloudTrace}
	if _, err := Setup(context.Background(), cfg, nil); err == nil {
		t.Fatalf("expected error for missing project id")
	}
}

func TestSetupInvalidExporter(t *testing.T) {
	cfg := Config{ServiceName: "svc", Exporter: ExporterType("invalid")}
	if _, err := Setup(context.Background(), cfg, nil); err == nil {
		t.Fatalf("expected error for invalid exporter")
	}
}

func TestSetupAcceptsResourceOptions(t *testing.T) {
	cfg := Config{ServiceName: "svc", ResourceAttrs: map[string]string{"foo": "bar"}}
	prov, err := Setup(context.Background(), cfg, nil, WithResourceOptions())
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	_ = prov.Shutdown(context.Background())
}

func TestHTTPHelpers(t *testing.T) {
	handler := HTTPHandler("op", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status ok, got %d", rec.Code)
	}

	transport := HTTPTransport(http.DefaultTransport)
	if transport == nil {
		t.Fatalf("expected transport")
	}
}

func TestGRPCHandlers(t *testing.T) {
	if GRPCServerHandler() == nil {
		t.Fatalf("expected server handler")
	}
	if GRPCClientHandler() == nil {
		t.Fatalf("expected client handler")
	}
}

func saveGlobal() func() {
	currentTP := otel.GetTracerProvider()
	currentProp := otel.GetTextMapPropagator()

	return func() {
		otel.SetTracerProvider(currentTP)
		otel.SetTextMapPropagator(currentProp)
	}
}

type noopLogger struct{}

func (noopLogger) Debug(context.Context, string, ...logx.Attr)        {}
func (noopLogger) Info(context.Context, string, ...logx.Attr)         {}
func (noopLogger) Warn(context.Context, string, ...logx.Attr)         {}
func (noopLogger) Error(context.Context, string, error, ...logx.Attr) {}
func (noopLogger) Fatal(context.Context, string, error, ...logx.Attr) {}
func (noopLogger) With(...logx.Attr) logx.Logger                      { return noopLogger{} }
