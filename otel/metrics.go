package otel

import (
	"context"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"
	runtimemetrics "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func configureMetrics(ctx context.Context, client *client, conf *config) {
	exp, err := otlpmetrichttp.New(ctx, otlpMetricOptions(conf)...)
	if err != nil {
		log.Err(err).Msgf("otlpmetricClient failed: %s", err)
		return
	}

	reader := sdkmetric.NewPeriodicReader(
		exp,
		sdkmetric.WithInterval(15*time.Second),
	)

	providerOptions := append(conf.metricOptions,
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(conf.newResource()),
	)
	provider := sdkmetric.NewMeterProvider(providerOptions...)

	otel.SetMeterProvider(provider)
	client.mp = provider

	if err := runtimemetrics.Start(); err != nil {
		log.Err(err).Msgf("runtimemetrics.Start failed")
	}
}

func otlpMetricOptions(conf *config) []otlpmetrichttp.Option {
	options := []otlpmetrichttp.Option{
		otlpmetrichttp.WithHeaders(conf.headers),
		otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression),
		otlpmetrichttp.WithTemporalitySelector(preferDeltaTemporalitySelector),
	}

	u, _ := url.Parse(conf.endpoint)
	if u != nil {
		options = append(options, otlpmetrichttp.WithEndpoint(u.Host))
	}

	if conf.tlsConf != nil {
		options = append(options, otlpmetrichttp.WithTLSClientConfig(conf.tlsConf))
	} else {
		if u != nil && (u.Scheme == "http" || u.Scheme == "unix") {
			options = append(options, otlpmetrichttp.WithInsecure())
		}
	}

	return options
}

func preferDeltaTemporalitySelector(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	switch kind {
	case sdkmetric.InstrumentKindCounter,
		sdkmetric.InstrumentKindObservableCounter,
		sdkmetric.InstrumentKindHistogram:
		return metricdata.DeltaTemporality
	default:
		return metricdata.CumulativeTemporality
	}
}
