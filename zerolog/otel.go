package zerolog

import (
	"context"

	"github.com/agoda-com/opentelemetry-go/otelzerolog"
	"github.com/agoda-com/opentelemetry-logs-go/exporters/otlp/otlplogs"
	"github.com/agoda-com/opentelemetry-logs-go/exporters/otlp/otlplogs/otlplogshttp"
	sdklogs "github.com/agoda-com/opentelemetry-logs-go/sdk/logs"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ggsrc/gopkg/env"
)

func init() {
	InitDefaultLogger()
}

func InitDefaultLogger() {
	ctx := context.Background()
	exporter, _ := otlplogs.NewExporter(ctx, otlplogs.WithClient(otlplogshttp.NewClient()))
	batchSize := 512
	if env.IsStaging() {
		batchSize = 10
	}
	loggerProvider := sdklogs.NewLoggerProvider(
		sdklogs.WithBatcher(
			exporter,
			// add following two options to ensure flush
			//sdklogs.WithBatchTimeout(5*time.Second),
			sdklogs.WithMaxExportBatchSize(batchSize),
		),
	)
	hook := otelzerolog.NewHook(loggerProvider)
	loggerVal := log.With().Caller().Logger()
	loggerVal = loggerVal.Hook(hook)
	zerolog.DefaultContextLogger = &loggerVal
}
