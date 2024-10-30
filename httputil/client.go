package httputil

import (
	"context"
	"net/http"
	"net/http/httptrace"

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
	res, err := t.RoundTripper.RoundTrip(r)
	if err != nil {
		span.RecordError(err)
		recordRequestAndResponse(ctx, span, r, res)
		span.End()
		return res, err
	}
	recordRequestAndResponse(ctx, span, r, res)
	span.End()
	return res, err
}

func recordRequestAndResponse(ctx context.Context, span trace.Span, req *http.Request, res *http.Response) {
	if req != nil {
		reqBody, err := readRequestBody(req)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to read request body")
		}
		span.SetAttributes(attribute.String("http.request.body", reqBody))
	}
	if res != nil {
		resBody, err := readResBody(res)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to read response body")
		}
		span.SetAttributes(attribute.String("http.response.body", resBody))
	}
}

func DefaultHttpClient(name string) *http.Client {
	transport := otelhttp.NewTransport(
		http.DefaultTransport,
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return name + "HTTP " + r.Method
		}),
		otelhttp.WithClientTrace(func(ctx context.Context) *httptrace.ClientTrace {
			return &httptrace.ClientTrace{}
		}),
	)
	return &http.Client{Transport: &Transport{Name: name, RoundTripper: transport}}
}
