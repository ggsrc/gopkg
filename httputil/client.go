package httputil

import (
	"context"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/ggsrc/gopkg/env"
	"github.com/ggsrc/gopkg/zerolog/log"
)

const name = "github.com/ggsrc/httputil"

var (
	tracer = otel.Tracer(name)
)

type Transport struct {
	Name string
	http.RoundTripper
}

func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	//ctx := injectRequest(r.Context(), r)
	//r2 := r.WithContext(ctx)
	if !env.IsStaging() {
		return t.RoundTripper.RoundTrip(r)
	}

	ctx, span := tracer.Start(r.Context(), t.Name+" "+r.Method)
	recordRequest(ctx, span, r)

	res, err := t.RoundTripper.RoundTrip(r)
	if err != nil {
		span.RecordError(err)
		recordResponse(ctx, span, res)
		span.End()
		return res, err
	}
	recordResponse(ctx, span, res)
	span.End()
	return res, err
}

func recordResponse(ctx context.Context, span trace.Span, res *http.Response) {
	if res != nil {
		resBody, err := readResBody(res)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to read response body")
		}
		span.SetAttributes(attribute.String("http.response.body", resBody))
	}
}

func recordRequest(ctx context.Context, span trace.Span, req *http.Request) {
	if req != nil {
		reqBody, err := readRequestBody(req)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to read request body")
		}
		span.SetAttributes(attribute.String("http.request.body", reqBody))
		span.SetAttributes(attribute.String("http.url", req.URL.String()))
	}
}

func NewDefaultHttpClient(name string) *http.Client {
	return &http.Client{Transport: NewTransport(name, http.DefaultTransport)}
}

func NewTransport(name string, base http.RoundTripper, opts ...otelhttp.Option) http.RoundTripper {
	opts = append([]otelhttp.Option{
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return name + " HTTP " + r.Method
		}),
	}, opts...)
	transport := otelhttp.NewTransport(
		base,
		opts...,
	)
	return &Transport{Name: name, RoundTripper: transport}
}
