package otel

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"math/rand"
	"net/url"
	"runtime"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func configureTracing(ctx context.Context, client *client, conf *config) {
	exp, err := otlptracehttp.New(ctx, otlpTraceOptions(conf)...)
	if err != nil {
		log.Err(err).Msgf("otlptrace.New failed")
		return
	}

	var opts []sdktrace.TracerProviderOption
	opts = append(opts, sdktrace.WithIDGenerator(newIDGenerator()))
	if res := conf.newResource(); res != nil {
		opts = append(opts, sdktrace.WithResource(res))
	}
	provider := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(provider)

	bspOptions := []sdktrace.BatchSpanProcessorOption{
		sdktrace.WithMaxQueueSize(queueSize()),
		sdktrace.WithMaxExportBatchSize(queueSize()),
		sdktrace.WithBatchTimeout(10 * time.Second),
		sdktrace.WithExportTimeout(10 * time.Second),
	}
	bspOptions = append(bspOptions, conf.bspOptions...)
	bsp := sdktrace.NewBatchSpanProcessor(exp, bspOptions...)
	provider.RegisterSpanProcessor(bsp)

	if conf.prettyPrint {
		exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			log.Warn().Err(err).Msg("stdouttrace.New failed")
		} else {
			provider.RegisterSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter))
		}
	}

	client.tp = provider
}

func otlpTraceOptions(conf *config) []otlptracehttp.Option {
	options := []otlptracehttp.Option{
		otlptracehttp.WithHeaders(conf.headers),
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
	}

	u, _ := url.Parse(conf.endpoint)
	if u != nil {
		options = append(options, otlptracehttp.WithEndpoint(u.Host))
	}

	if conf.tlsConf != nil {
		options = append(options, otlptracehttp.WithTLSClientConfig(conf.tlsConf))
	} else {
		if u != nil && (u.Scheme == "http" || u.Scheme == "unix") {
			options = append(options, otlptracehttp.WithInsecure())
		}
	}
	return options
}

func queueSize() int {
	const min = 1000
	const max = 16000

	n := (runtime.GOMAXPROCS(0) / 2) * 1000
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

//------------------------------------------------------------------------------

const spanIDPrec = int64(time.Millisecond)

type idGenerator struct {
	sync.Mutex
	randSource *rand.Rand
}

func newIDGenerator() *idGenerator {
	gen := &idGenerator{}
	var rngSeed int64
	_ = binary.Read(cryptorand.Reader, binary.LittleEndian, &rngSeed)
	gen.randSource = rand.New(rand.NewSource(rngSeed)) //nolint:gosec
	return gen
}

var _ sdktrace.IDGenerator = (*idGenerator)(nil)

// NewIDs returns a new trace and span ID.
func (gen *idGenerator) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	unixNano := time.Now().UnixNano()

	gen.Lock()
	defer gen.Unlock()

	tid := trace.TraceID{}
	binary.BigEndian.PutUint64(tid[:8], uint64(unixNano)) //nolint:gosec
	_, _ = gen.randSource.Read(tid[8:])

	sid := trace.SpanID{}
	binary.BigEndian.PutUint32(sid[:4], uint32(unixNano/spanIDPrec)) //nolint:gosec
	_, _ = gen.randSource.Read(sid[4:])

	return tid, sid
}

// NewSpanID returns a ID for a new span in the trace with traceID.
func (gen *idGenerator) NewSpanID(ctx context.Context, traceID trace.TraceID) trace.SpanID {
	unixNano := time.Now().UnixNano()

	gen.Lock()
	defer gen.Unlock()

	sid := trace.SpanID{}
	binary.BigEndian.PutUint32(sid[:4], uint32(unixNano/spanIDPrec)) //nolint:gosec
	_, _ = gen.randSource.Read(sid[4:])

	return sid
}
