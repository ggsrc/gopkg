package otel

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/ggsrc/gopkg/env"
)

type config struct {
	endpoint string
	headers  map[string]string

	// Common options

	resourceAttributes []attribute.KeyValue
	resourceDetectors  []resource.Detector
	resource           *resource.Resource

	tlsConf *tls.Config

	// Tracing options
	tracingEnabled    bool
	textMapPropagator propagation.TextMapPropagator
	bspOptions        []sdktrace.BatchSpanProcessorOption
	prettyPrint       bool

	// Metrics options
	metricsEnabled bool
	metricOptions  []metric.Option

	// Logging options
	loggingEnabled bool
	loggerProvider *sdklog.LoggerProvider
}

func newConfig(opts []Option) *config {
	conf := &config{
		headers:        map[string]string{},
		tracingEnabled: true,
		metricsEnabled: true,
		loggingEnabled: true,
	}

	// 解析环境变量
	if v, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT"); ok {
		conf.endpoint = v
	}

	if v, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_HEADERS"); ok {
		var err error
		for _, header := range strings.Split(v, ",") {
			rawKey, rawVal, found := strings.Cut(header, "=")
			if !found {
				err = errors.Join(err, fmt.Errorf("invalid header: %s", header))
				continue
			}
			escKey, e := url.PathUnescape(rawKey)
			if e != nil {
				err = errors.Join(err, fmt.Errorf("invalid header key: %s", rawKey))
				continue
			}
			escVal, e := url.PathUnescape(rawVal)
			if e != nil {
				err = errors.Join(err, fmt.Errorf("invalid header value: %s", rawVal))
				continue
			}
			conf.headers[strings.TrimSpace(escKey)] = strings.TrimSpace(escVal)
		}
		if err != nil {
			log.Error().Msgf("failed to parse [OTEL_EXPORTER_OTLP_HEADERS]: %s", v)
		}
	}

	for _, opt := range opts {
		opt.apply(conf)
	}

	return conf
}

func (conf *config) newResource() *resource.Resource {
	if conf.resource != nil {
		if len(conf.resourceAttributes) > 0 {
			log.Printf("WithResource overrides WithResourceAttributes (discarding %v)",
				conf.resourceAttributes)
		}
		if len(conf.resourceDetectors) > 0 {
			log.Printf("WithResource overrides WithResourceDetectors (discarding %v)",
				conf.resourceDetectors)
		}
		return conf.resource
	}

	ctx := context.TODO()

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithDetectors(conf.resourceDetectors...),
		resource.WithAttributes(conf.resourceAttributes...),
		resource.WithAttributes(attribute.String("env", env.Env())),
		resource.WithAttributes(attribute.String("service.name", env.ServiceName())),
		resource.WithAttributes(attribute.String("service.version", env.ServiceVersion())),
	)
	if err != nil {
		otel.Handle(err)
		return resource.Environment()
	}
	return res
}

//------------------------------------------------------------------------------

type Option interface {
	apply(conf *config)
}

type option func(conf *config)

func (fn option) apply(conf *config) {
	fn(conf)
}

var _ Option = (*option)(nil)

// WithEndpoint configures the endpoint to send telemetry data to.
func WithEndpoint(endpoint string) Option {
	return option(func(conf *config) {
		conf.endpoint = endpoint
	})
}

// WithHeaders configures headers to send with telemetry data.
func WithHeaders(headers map[string]string) Option {
	return option(func(conf *config) {
		if headers == nil {
			return
		}
		if conf.headers == nil {
			conf.headers = map[string]string{}
		}
		for k, v := range headers {
			conf.headers[k] = v
		}
	})
}

// WithServiceName configures `service.name` resource attribute.
func WithServiceName(serviceName string) Option {
	return option(func(conf *config) {
		attr := semconv.ServiceNameKey.String(serviceName)
		conf.resourceAttributes = append(conf.resourceAttributes, attr)
	})
}

// WithServiceVersion configures `service.version` resource attribute, for example, `1.0.0`.
func WithServiceVersion(serviceVersion string) Option {
	return option(func(conf *config) {
		attr := semconv.ServiceVersionKey.String(serviceVersion)
		conf.resourceAttributes = append(conf.resourceAttributes, attr)
	})
}

// WithDeploymentEnvironment configures `deployment.environment` resource attribute,
// for example, `production`.
func WithDeploymentEnvironment(env string) Option {
	return option(func(conf *config) {
		attr := semconv.DeploymentEnvironmentKey.String(env)
		conf.resourceAttributes = append(conf.resourceAttributes, attr)
	})
}

// WithResourceAttributes configures resource attributes that describe an entity that produces
// telemetry, for example, such attributes as host.name, service.name, etc.
//
// The default is to use `OTEL_RESOURCE_ATTRIBUTES` env var, for example,
// `OTEL_RESOURCE_ATTRIBUTES=service.name=myservice,service.version=1.0.0`.
func WithResourceAttributes(attrs ...attribute.KeyValue) Option {
	return option(func(conf *config) {
		conf.resourceAttributes = append(conf.resourceAttributes, attrs...)
	})
}

// WithResourceDetectors adds detectors to be evaluated for the configured resource.
func WithResourceDetectors(detectors ...resource.Detector) Option {
	return option(func(conf *config) {
		conf.resourceDetectors = append(conf.resourceDetectors, detectors...)
	})
}

// WithResource configures a resource that describes an entity that produces telemetry,
// for example, such attributes as host.name and service.name. All produced spans and metrics
// will have these attributes.
//
// WithResource overrides and replaces any other resource attributes.
func WithResource(resource *resource.Resource) Option {
	return option(func(conf *config) {
		conf.resource = resource
	})
}

func WithTLSConfig(tlsConf *tls.Config) Option {
	return option(func(conf *config) {
		conf.tlsConf = tlsConf
	})
}

//------------------------------------------------------------------------------

type TracingOption interface {
	Option
	tracing()
}

type tracingOption func(conf *config)

func (fn tracingOption) apply(conf *config) {
	fn(conf)
}

func (fn tracingOption) tracing() {}

var _ TracingOption = (*tracingOption)(nil)

// WithTracingEnabled can be used to enable/disable tracing.
func WithTracingEnabled(on bool) TracingOption {
	return tracingOption(func(conf *config) {
		conf.tracingEnabled = on
	})
}

// WithTracingDisabled disables tracing.
func WithTracingDisabled() TracingOption {
	return WithTracingEnabled(false)
}

// WithPropagator sets the global TextMapPropagator used by OpenTelemetry.
// The default is propagation.TraceContext and propagation.Baggage.
func WithPropagator(propagator propagation.TextMapPropagator) TracingOption {
	return tracingOption(func(conf *config) {
		conf.textMapPropagator = propagator
	})
}

// WithTextMapPropagator is an alias for WithPropagator.
func WithTextMapPropagator(propagator propagation.TextMapPropagator) TracingOption {
	return WithPropagator(propagator)
}

// WithPrettyPrintSpanExporter adds a span exproter that prints spans to stdout.
// It is useful for debugging or demonstration purposes.
func WithPrettyPrintSpanExporter() TracingOption {
	return tracingOption(func(conf *config) {
		conf.prettyPrint = true
	})
}

// WithBatchSpanProcessorOption specifies options used to created BatchSpanProcessor.
func WithBatchSpanProcessorOption(opts ...sdktrace.BatchSpanProcessorOption) TracingOption {
	return tracingOption(func(conf *config) {
		conf.bspOptions = append(conf.bspOptions, opts...)
	})
}

//------------------------------------------------------------------------------

type MetricsOption interface {
	Option
	metrics()
}

type metricsOption func(conf *config)

var _ MetricsOption = (*metricsOption)(nil)

func (fn metricsOption) apply(conf *config) {
	fn(conf)
}

func (fn metricsOption) metrics() {}

// WithMetricsEnabled can be used to enable/disable metrics.
func WithMetricsEnabled(on bool) MetricsOption {
	return metricsOption(func(conf *config) {
		conf.metricsEnabled = on
	})
}

// WithMetricsDisabled disables metrics.
func WithMetricsDisabled() MetricsOption {
	return WithMetricsEnabled(false)
}

func WithMetricOption(options ...metric.Option) MetricsOption {
	return metricsOption(func(conf *config) {
		conf.metricOptions = append(conf.metricOptions, options...)
	})
}

//------------------------------------------------------------------------------

type LoggingOption interface {
	Option
	logging()
}

type loggingOption func(conf *config)

var _ LoggingOption = (*loggingOption)(nil)

func (fn loggingOption) apply(conf *config) {
	fn(conf)
}

func (fn loggingOption) logging() {}

// WithLoggingDisabled disables logging.
func WithLoggingDisabled() LoggingOption {
	return WithLoggingEnabled(false)
}

// WithLoggingEnabled can be used to enable/disable logging.
func WithLoggingEnabled(on bool) LoggingOption {
	return loggingOption(func(conf *config) {
		conf.loggingEnabled = on
	})
}

// WithLoggerProvider overwrites the default logger provider.
//
// When this option is used, you might need to call otel.SetLoggerProvider
// to register the provider as the global trace provider.
func WithLoggerProvider(provider *sdklog.LoggerProvider) LoggingOption {
	return loggingOption(func(conf *config) {
		conf.loggerProvider = provider
	})
}
