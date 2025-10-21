package otelx

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// HTTPHandler wraps the provided handler with OpenTelemetry instrumentation.
func HTTPHandler(operation string, handler http.Handler, opts ...otelhttp.Option) http.Handler {
	if operation == "" {
		operation = "http.request"
	}
	return otelhttp.NewHandler(handler, operation, opts...)
}

// HTTPTransport wraps the given RoundTripper with OpenTelemetry instrumentation.
func HTTPTransport(base http.RoundTripper, opts ...otelhttp.Option) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return otelhttp.NewTransport(base, opts...)
}
