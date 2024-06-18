package log_test

import (
	"context"
	otelzerolog "github.com/ggsrc/gopkg/zerolog"
	"github.com/ggsrc/gopkg/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/uptrace/uptrace-go/uptrace"
	"go.opentelemetry.io/otel/trace"
	"testing"
)

func TestLog(t *testing.T) {
	uptrace.ConfigureOpentelemetry()
	otelzerolog.InitLogger(true)
	traceID, err := trace.TraceIDFromHex("17d023447a330049126f26a9996249c0")
	assert.NoError(t, err)
	spaceID, err := trace.SpanIDFromHex("fd1054dfa5132df5")
	assert.NoError(t, err)
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spaceID,
		TraceFlags: trace.FlagsSampled,
		TraceState: trace.TraceState{},
		Remote:     false,
	}))

	logger := log.With().Ctx(ctx).Logger()
	logger.Info().Msg("hello world")
}
