package otel

import (
	"context"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

const (
	SchemeHttp = "http"
	SchemeUnix = "unix"
)

func configureLogging(ctx context.Context, client *client, conf *config) {
	if conf.loggerProvider != nil {
		global.SetLoggerProvider(conf.loggerProvider)
		client.lp = conf.loggerProvider
		return
	}
	exp, err := otlploghttp.New(ctx, otlpLoggerOptions(conf)...)
	if err != nil {
		log.Err(err).Msgf("otlploghttp.New failed")
		return
	}

	bspOptions := []sdklog.BatchProcessorOption{
		sdklog.WithMaxQueueSize(queueSize()),
		sdklog.WithExportMaxBatchSize(queueSize()),
		sdklog.WithExportInterval(10 * time.Second),
		sdklog.WithExportTimeout(10 * time.Second),
	}
	bsp := sdklog.NewBatchProcessor(exp, bspOptions...)

	var opts []sdklog.LoggerProviderOption
	opts = append(opts, sdklog.WithProcessor(bsp))
	if res := conf.newResource(); res != nil {
		opts = append(opts, sdklog.WithResource(res))
	}

	provider := sdklog.NewLoggerProvider(opts...)
	global.SetLoggerProvider(provider)
	client.lp = provider
}

func otlpLoggerOptions(conf *config) []otlploghttp.Option {
	options := []otlploghttp.Option{
		otlploghttp.WithHeaders(conf.headers),
		otlploghttp.WithCompression(otlploghttp.GzipCompression),
	}

	u, _ := url.Parse(conf.endpoint)
	if u != nil {
		options = append(options, otlploghttp.WithEndpoint(u.Host))
	}

	if conf.tlsConf != nil {
		options = append(options, otlploghttp.WithTLSClientConfig(conf.tlsConf))
	} else {
		if u != nil && (u.Scheme == SchemeHttp || u.Scheme == SchemeUnix) {
			options = append(options, otlploghttp.WithInsecure())
		}
	}

	return options
}
