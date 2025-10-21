package otelx

import (
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
)

type setupOptions struct {
	global       bool
	propagator   propagation.TextMapPropagator
	resourceOpts []resource.Option
	samplerHook  func(float64)
}

// Option customises Setup behaviour.
type Option func(*setupOptions)

// WithGlobal registers the created provider & propagator as global defaults.
func WithGlobal() Option {
	return func(o *setupOptions) {
		o.global = true
	}
}

// WithPropagator overrides the default propagator returned by Setup.
func WithPropagator(p propagation.TextMapPropagator) Option {
	return func(o *setupOptions) {
		o.propagator = p
	}
}

// WithResourceOptions appends additional resource options when constructing service Resource.
func WithResourceOptions(opts ...resource.Option) Option {
	return func(o *setupOptions) {
		o.resourceOpts = append(o.resourceOpts, opts...)
	}
}

func withSamplerHook(hook func(float64)) Option {
	return func(o *setupOptions) {
		o.samplerHook = hook
	}
}
