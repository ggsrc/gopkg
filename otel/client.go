package otel

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const dummySpanName = "__dummy__"

// client represents client.
type client struct {
	tracer trace.Tracer

	tp *sdktrace.TracerProvider
	mp *sdkmetric.MeterProvider
	lp *sdklog.LoggerProvider
}

func newClient() *client {
	return &client{
		tracer: otel.Tracer("otel-go"),
	}
}

func (c *client) Shutdown(ctx context.Context) (lastErr error) {
	if c.tp != nil {
		if err := c.tp.Shutdown(ctx); err != nil {
			lastErr = err
		}
		c.tp = nil
	}
	if c.mp != nil {
		if err := c.mp.Shutdown(ctx); err != nil {
			lastErr = err
		}
		c.mp = nil
	}
	if c.lp != nil {
		if err := c.lp.Shutdown(ctx); err != nil {
			lastErr = err
		}
		c.lp = nil
	}
	return lastErr
}

func (c *client) ForceFlush(ctx context.Context) (lastErr error) {
	if c.tp != nil {
		if err := c.tp.ForceFlush(ctx); err != nil {
			lastErr = err
		}
	}
	if c.mp != nil {
		if err := c.mp.ForceFlush(ctx); err != nil {
			lastErr = err
		}
	}
	if c.lp != nil {
		if err := c.lp.ForceFlush(ctx); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// TraceURL returns the trace URL for the span.
func (c *client) TraceURL(span trace.Span) string {
	sctx := span.SpanContext()
	return fmt.Sprintf("/traces/%s?span_id=%s", sctx.TraceID(), sctx.SpanID().String())
}

// ReportError reports an error as a span event creating a dummy span if necessary.
func (c *client) ReportError(ctx context.Context, err error, opts ...trace.EventOption) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		_, span = c.tracer.Start(ctx, dummySpanName)
		defer span.End()
	}

	span.RecordError(err, opts...)
}

// ReportPanic is used with defer to report panics.
func (c *client) ReportPanic(ctx context.Context, val any) {
	c.reportPanic(ctx, val)
	// Force flush since we are about to exit on panic.
	if c.tp != nil {
		_ = c.tp.ForceFlush(ctx)
	}
}

func (c *client) reportPanic(ctx context.Context, val interface{}) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		_, span = c.tracer.Start(ctx, dummySpanName)
		defer span.End()
	}

	stackTrace := make([]byte, 2048)
	n := runtime.Stack(stackTrace, false)

	span.AddEvent(
		"log",
		trace.WithAttributes(
			attribute.String("log.severity", "panic"),
			attribute.String("log.message", fmt.Sprint(val)),
			attribute.String("exception.stackstrace", string(stackTrace[:n])),
		),
	)
}

// ConfigureOpentelemetry configures OpenTelemetry to export data to otlp.
// By default it:
//   - creates tracer provider;
//   - registers span exporter;
//   - sets tracecontext + baggage composite context propagator.
//
// You can use OTEL_DISABLED env var to completely skip configuration.
func ConfigureOpenTelemetry(opts ...Option) {
	if _, ok := os.LookupEnv("OTEL_DISABLED"); ok {
		return
	}

	ctx := context.TODO()
	conf := newConfig(opts)

	if !conf.tracingEnabled && !conf.metricsEnabled && !conf.loggingEnabled {
		return
	}

	client := newClient()

	configurePropagator(conf)
	if conf.tracingEnabled {
		configureTracing(ctx, client, conf)
	}
	if conf.metricsEnabled {
		configureMetrics(ctx, client, conf)
	}
	if conf.loggingEnabled {
		configureLogging(ctx, client, conf)
	}

	atomicClient.Store(client)
}

func configurePropagator(conf *config) {
	textMapPropagator := conf.textMapPropagator
	if textMapPropagator == nil {
		textMapPropagator = propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		)
	}
	otel.SetTextMapPropagator(textMapPropagator)
}

//------------------------------------------------------------------------------

var (
	fallbackClient = newClient()
	atomicClient   atomic.Value
)

func activeClient() *client {
	v := atomicClient.Load()
	if v == nil {
		return fallbackClient
	}
	return v.(*client)
}

func TraceURL(span trace.Span) string {
	return activeClient().TraceURL(span)
}

func ReportError(ctx context.Context, err error, opts ...trace.EventOption) {
	activeClient().ReportError(ctx, err, opts...)
}

func ReportPanic(ctx context.Context, val any) {
	activeClient().ReportPanic(ctx, val)
}

func Shutdown(ctx context.Context) error {
	return activeClient().Shutdown(ctx)
}

func ForceFlush(ctx context.Context) error {
	return activeClient().ForceFlush(ctx)
}

func TracerProvider() *sdktrace.TracerProvider {
	return activeClient().tp
}
