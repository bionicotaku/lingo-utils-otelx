package otelx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	logx "github.com/bionicotaku/lingo-utils-logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
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
	prov, err := Setup(context.Background(), Config{ServiceName: "svc", SamplingRatio: Float64(0.05)}, nil)
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

func TestSetupUsesDefaultSamplingRatioWhenUnset(t *testing.T) {
	restore := saveGlobal()
	defer restore()

	var observed float64
	prov, err := Setup(context.Background(), Config{ServiceName: "svc"}, nil, withSamplerHook(func(v float64) {
		observed = v
	}))
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if observed != DefaultSamplingRatio {
		t.Fatalf("expected default sampling ratio %v, got %v", DefaultSamplingRatio, observed)
	}
	_ = prov.Shutdown(context.Background())
}

func TestSetupAllowsZeroSamplingRatio(t *testing.T) {
	restore := saveGlobal()
	defer restore()

	var observed float64 = -1
	prov, err := Setup(context.Background(), Config{
		ServiceName:    "svc",
		SamplingRatio:  Float64(0),
		Exporter:       ExporterStdout,
		ServiceVersion: "test",
	}, nil, withSamplerHook(func(v float64) {
		observed = v
	}))
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if observed != 0 {
		t.Fatalf("expected sampling ratio 0, got %v", observed)
	}
	tracer := prov.TP.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "zero_sampling")
	if span.IsRecording() {
		t.Fatalf("expected span to be non-recording when sampling ratio is zero")
	}
	span.End()
	_ = prov.Shutdown(ctx)
}

func TestSetupIncludesDefaultResourceDetectors(t *testing.T) {
	restore := saveGlobal()
	defer restore()

	prov, err := Setup(context.Background(), Config{ServiceName: "svc", SamplingRatio: Float64(1)}, nil)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	capture := &resourceCapture{}
	prov.TP.RegisterSpanProcessor(capture)

	tracer := prov.TP.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "with-default-resource")
	span.End()
	if err := prov.TP.ForceFlush(ctx); err != nil {
		t.Fatalf("force flush failed: %v", err)
	}

	res := capture.Resource()
	if res == nil {
		t.Fatalf("expected resource to be captured")
	}
	if !hasAttribute(res, semconv.ServiceNameKey, "svc") {
		t.Fatalf("expected service name attribute to be present")
	}
	if !hasAttribute(res, semconv.TelemetrySDKLanguageKey, "go") {
		t.Fatalf("expected telemetry sdk language attribute to be present")
	}

	_ = prov.Shutdown(ctx)
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

type resourceCapture struct {
	mu  sync.Mutex
	res *resource.Resource
}

func (c *resourceCapture) OnStart(context.Context, sdktrace.ReadWriteSpan) {}

func (c *resourceCapture) OnEnd(span sdktrace.ReadOnlySpan) {
	c.mu.Lock()
	c.res = span.Resource()
	c.mu.Unlock()
}

func (c *resourceCapture) Shutdown(context.Context) error { return nil }

func (c *resourceCapture) ForceFlush(context.Context) error { return nil }

func (c *resourceCapture) Resource() *resource.Resource {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.res
}

func hasAttribute(res *resource.Resource, key attribute.Key, expected string) bool {
	if res == nil {
		return false
	}
	for _, attr := range res.Attributes() {
		if attr.Key == key && attr.Value.AsString() == expected {
			return true
		}
	}
	return false
}
