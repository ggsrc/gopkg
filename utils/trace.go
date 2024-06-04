package utils

import (
	"context"

	"go.opentelemetry.io/otel"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/ggsrc/gopkg/env"
)

const name = "github.com/NFTGalaxy/app"

var (
	tracer = otel.Tracer(name)
)

func StartTrace(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	opts = append(opts,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			semconv.ServiceNameKey.String(env.ServiceName()),
		),
	)
	ctx, span := tracer.Start(ctx, spanName, opts...)
	return ctx, span
}
