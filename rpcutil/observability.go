// observability.go — panic recover interceptors / subapp logger interceptors /
// safe background goroutine (observability.Go) / framework metric vars +
// grpc_prometheus sync.Once register.
//
// 详见:
//   - mega-project/docs/11-framework-startup-shutdown.md §三 (panic recover)
//   - mega-project/docs/11-framework-startup-shutdown.md §四 (observability.Go)
//   - mega-project/docs/12-framework-observability.md §一 (subapp logger interceptors)
//   - mega-project/docs/12-framework-observability.md §二 (metric registry +
//     grpc_prometheus.sync.Once)
//
// 文件所有权：Wave 2 agent E.
// 不要把 cron_* / depUnhealthyCounter 搬到本文件 —— 它们继续住在
// cron.go / health.go 各自的 init() 里。
package rpcutil

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gopkg_zerolog "github.com/ggsrc/gopkg/zerolog"
)

// ---- framework metric vars (consolidated) -----------------------------
//
// 所有 framework 层 metric var 集中在本文件，单一 init() 统一注册到
// prometheus.DefaultRegisterer。包含 panic / goroutine / shutdown 通用 metric，
// 以及 cron / dep health 域专用 metric — 它们在各自文件的方法体里被使用
// (cron.go wrapCronFn / health.go readinessHandler) 但声明集中在这里。
//
// 例外：仅一处 health.go 的 depUnhealthyCounter 仍在 health.go 内声明
// (Wave 1 期间 Agent C 独立 init，单独 var 不破坏全局排序；保留以避免无谓 diff)。

var (
	// panicCounter records panics recovered by framework interceptors /
	// middleware. Labels: subapp, site ("unary" / "stream" / "http"),
	// method (gRPC FullMethod or HTTP path).
	panicCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mega_panic_total",
			Help: "Number of panics recovered by framework interceptors/middleware.",
		},
		[]string{"subapp", "site", "method"},
	)

	// goroutineFailCounter records non-nil error returns from background
	// goroutines launched via MultiResource.Go.
	goroutineFailCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mega_goroutine_fail_total",
			Help: "Number of background goroutine failures (non-nil error returns).",
		},
		[]string{"subapp", "name"},
	)

	// goroutinePanicCounter records panics in background goroutines.
	goroutinePanicCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mega_goroutine_panic_total",
			Help: "Number of panics recovered in background goroutines.",
		},
		[]string{"subapp", "name"},
	)

	// shutdownAbortCounter records timeouts during graceful shutdown.
	// Used by Wave 4 startup.go Stop().
	shutdownAbortCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mega_shutdown_aborted_total",
			Help: "Number of shutdown phases aborted due to timeout.",
		},
		[]string{"subapp", "phase"},
	)

	// ---- cron metrics ---- referenced by cron.go wrapCronFn ------------

	cronPanicCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mega_cron_panic_total",
			Help: "Number of times a cron job panicked (recovered).",
		},
		[]string{"subapp", "job"},
	)

	cronErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mega_cron_error_total",
			Help: "Number of times a cron job returned a non-nil error.",
		},
		[]string{"subapp", "job"},
	)

	cronDurationHist = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "mega_cron_duration_seconds",
			Help: "Cron job execution duration in seconds. Buckets span 100ms..30min, " +
				"covering typical sub-second probes through batch jobs. DefBuckets (5ms..10s) " +
				"would land every minute-scale job in +Inf, breaking histogram_quantile.",
			// Hand-picked buckets aligned to operational job sizes (PR #115 Round-7 review P-004/O-002).
			Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 180, 600, 1800},
		},
		[]string{"subapp", "job"},
	)

	// ---- request metrics ---- per-subapp HTTP/gRPC counters ------------

	grpcRequestCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mega_grpc_request_total",
			Help: "Number of gRPC requests handled, labeled by subapp + method + code.",
		},
		[]string{"subapp", "method", "code"},
	)

	httpRequestCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mega_http_request_total",
			Help: "Number of HTTP requests handled, labeled by subapp + path + method + status.",
		},
		[]string{"subapp", "path", "method", "status"},
	)
)

func init() {
	prometheus.MustRegister(
		panicCounter,
		goroutineFailCounter,
		goroutinePanicCounter,
		shutdownAbortCounter,
		cronPanicCounter,
		cronErrorCounter,
		cronDurationHist,
		grpcRequestCounter,
		httpRequestCounter,
	)
}

// ---- background goroutine backoff knobs (overridable in tests) -------
//
// Production: 1s initial, 60s max (matches docs/11 §四). Tests can shrink
// these to milliseconds via t.Cleanup to keep retry-loop coverage fast.

var (
	goroutineInitialBackoff = 1 * time.Second
	goroutineMaxBackoff     = 60 * time.Second
)

// ---- observability.Go — safe background goroutine --------------------

// Go starts a background goroutine with ctx / panic recover / exponential
// backoff restart. fn returning nil exits the goroutine naturally; fn
// returning an error or panicking causes a restart up to goroutineMaxBackoff.
//
// The goroutine's ctx is derived from m.rootCtx (the lifecycle ctx passed to
// NewMultiResource), NOT context.Background — so when the caller's ctx is
// canceled (typical: signal.NotifyContext SIGTERM) OR Stop() runs, the
// retry loop exits cleanly. Addresses PR #115 review G2 (gemini).
//
// See mega-project/docs/11-framework-startup-shutdown.md §四.
func (m *MultiResource) Go(subapp, name string, fn func(ctx context.Context) error) {
	parent := m.rootCtx
	if parent == nil {
		// NewMultiResource always populates rootCtx; the nil branch is a
		// defensive fallback (e.g. tests that build *MultiResource by hand).
		parent = context.Background()
	}
	ctx := m.subAppCtxFromParent(parent, subapp)
	go m.runGoroutineWithRetry(ctx, subapp, name, fn)
}

func (m *MultiResource) runGoroutineWithRetry(
	ctx context.Context,
	subapp, name string,
	fn func(context.Context) error,
) {
	backoff := goroutineInitialBackoff
	maxBackoff := goroutineMaxBackoff

	for {
		if ctx.Err() != nil {
			return
		}

		err := m.runOnce(ctx, subapp, name, fn)
		if err == nil {
			return
		}

		log.Error().
			Err(err).
			Str("subapp", subapp).
			Str("goroutine", name).
			Dur("backoff", backoff).
			Msg("background goroutine failed; retrying")
		goroutineFailCounter.WithLabelValues(subapp, name).Inc()

		// G5: time.After in a retry loop leaks a Timer until it fires (since
		// the ctx.Done() branch wins, the unfired Timer's runtime resources
		// stay allocated). Use NewTimer + Stop so cancellation releases the
		// resources immediately.
		t := time.NewTimer(backoff)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (m *MultiResource) runOnce(
	ctx context.Context,
	subapp, name string,
	fn func(context.Context) error,
) (err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			log.Error().
				Str("subapp", subapp).
				Str("goroutine", name).
				Interface("panic", r).
				Bytes("stack", stack).
				Msg("background goroutine panic recovered")
			goroutinePanicCounter.WithLabelValues(subapp, name).Inc()
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return fn(ctx)
}

// ---- panic recover interceptors --------------------------------------

// panicRecoverUnaryInterceptor returns a gRPC unary interceptor that recovers
// from panics, increments panicCounter, and returns codes.Internal.
func (m *MultiResource) panicRecoverUnaryInterceptor(subapp string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context, req interface{},
		info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				log.Error().
					Str("subapp", subapp).
					Str("method", info.FullMethod).
					Interface("panic", r).
					Bytes("stack", stack).
					Msg("panic recovered (unary)")
				panicCounter.WithLabelValues(subapp, "unary", info.FullMethod).Inc()
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// panicRecoverStreamInterceptor returns a gRPC stream interceptor that recovers
// from panics, increments panicCounter, and returns codes.Internal.
func (m *MultiResource) panicRecoverStreamInterceptor(subapp string) grpc.StreamServerInterceptor {
	return func(
		srv interface{}, ss grpc.ServerStream,
		info *grpc.StreamServerInfo, handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				log.Error().
					Str("subapp", subapp).
					Str("method", info.FullMethod).
					Interface("panic", r).
					Bytes("stack", stack).
					Msg("panic recovered (stream)")
				panicCounter.WithLabelValues(subapp, "stream", info.FullMethod).Inc()
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(srv, ss)
	}
}

// panicRecoverHttpMiddleware returns a gin middleware that recovers from
// panics, increments panicCounter, and aborts with HTTP 500.
func (m *MultiResource) panicRecoverHttpMiddleware(subapp string) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				// PR #115 Round-10 M3: gin's c.FullPath() returns "" for
				// unmatched routes; map to "unknown" to stay consistent with
				// metricHttpMiddleware's label values (so joining panicCounter
				// against httpRequestCounter on `path` lines up).
				path := c.FullPath()
				if path == "" {
					path = "unknown"
				}
				log.Error().
					Str("subapp", subapp).
					Str("path", path).
					Interface("panic", r).
					Bytes("stack", stack).
					Msg("panic recovered (http)")
				panicCounter.WithLabelValues(subapp, "http", path).Inc()
				c.AbortWithStatusJSON(http.StatusInternalServerError,
					gin.H{"error": "internal server error"})
			}
		}()
		c.Next()
	}
}

// ---- subapp logger interceptors --------------------------------------

// subappLoggerUnaryInterceptor injects a sub-app aware logger into ctx
// (via gopkg_zerolog.WithSubApp and logger.WithContext).
func (m *MultiResource) subappLoggerUnaryInterceptor(subapp string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context, req interface{},
		info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
	) (interface{}, error) {
		ctx = m.injectSubAppCtx(ctx, subapp)
		return handler(ctx, req)
	}
}

// subappLoggerStreamInterceptor injects a sub-app aware logger into the
// server stream's context.
func (m *MultiResource) subappLoggerStreamInterceptor(subapp string) grpc.StreamServerInterceptor {
	return func(
		srv interface{}, ss grpc.ServerStream,
		info *grpc.StreamServerInfo, handler grpc.StreamHandler,
	) error {
		newCtx := m.injectSubAppCtx(ss.Context(), subapp)
		return handler(srv, &wrappedServerStream{ServerStream: ss, ctx: newCtx})
	}
}

// subappLoggerHttpMiddleware injects a sub-app aware logger into the
// request context for downstream gin handlers.
func (m *MultiResource) subappLoggerHttpMiddleware(subapp string) gin.HandlerFunc {
	return func(c *gin.Context) {
		newCtx := m.injectSubAppCtx(c.Request.Context(), subapp)
		c.Request = c.Request.WithContext(newCtx)
		c.Next()
	}
}

// injectSubAppCtx wraps ctx with the subapp marker and a logger that carries
// the "subapp" (and optionally "legacy_app") field. m.rootLogger may be nil
// during early test setups; fall back to the global zerolog/log.Logger.
func (m *MultiResource) injectSubAppCtx(ctx context.Context, subapp string) context.Context {
	ctx = gopkg_zerolog.WithSubApp(ctx, subapp)
	base := m.rootLogger
	if base == nil {
		base = &log.Logger
	}
	builder := base.With().Str("subapp", subapp)
	if m.migrationMode {
		builder = builder.Str("legacy_app", "galxe-"+subapp)
	}
	l := builder.Logger()
	return l.WithContext(ctx)
}

// ---- metric interceptors (subapp-labeled request counters) -----------

// metricUnaryInterceptor increments mega_grpc_request_total{subapp,method,code}
// on every unary call. Pairs with grpc_prometheus's default latency
// histogram (enabled in ensureGrpcPrometheus).
func (m *MultiResource) metricUnaryInterceptor(subapp string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context, req interface{},
		info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
	) (interface{}, error) {
		resp, err := handler(ctx, req)
		grpcRequestCounter.WithLabelValues(subapp, info.FullMethod, status.Code(err).String()).Inc()
		return resp, err
	}
}

// metricStreamInterceptor is the stream counterpart of metricUnaryInterceptor.
// Records exactly once on stream completion.
func (m *MultiResource) metricStreamInterceptor(subapp string) grpc.StreamServerInterceptor {
	return func(
		srv interface{}, ss grpc.ServerStream,
		info *grpc.StreamServerInfo, handler grpc.StreamHandler,
	) error {
		err := handler(srv, ss)
		grpcRequestCounter.WithLabelValues(subapp, info.FullMethod, status.Code(err).String()).Inc()
		return err
	}
}

// metricHttpMiddleware increments mega_http_request_total
// {subapp,path,method,status} after the handler chain completes.
func (m *MultiResource) metricHttpMiddleware(subapp string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		httpRequestCounter.WithLabelValues(
			subapp,
			path,
			c.Request.Method,
			strconv.Itoa(c.Writer.Status()),
		).Inc()
	}
}

// ---- grpc_prometheus sync.Once register ------------------------------

// grpcPromRegisterOnce ensures grpc_prometheus's global side-effect
// (DefaultServerMetrics registration + handling-time histogram) runs at
// most once across the whole process. Without this guard, a second
// MultiResource (e.g. in tests) panics inside prometheus.MustRegister.
var grpcPromRegisterOnce sync.Once

// ensureGrpcPrometheus enables grpc_prometheus's handling-time histogram
// exactly once. Safe to call from every GrpcServerOn invocation.
//
// Note: grpc_prometheus.DefaultServerMetrics is already registered with
// the prometheus default registry inside grpc_prometheus's own init();
// re-registering it here would panic with "already registered". Hence
// this Once is reserved for the histogram (which is opt-in).
func (m *MultiResource) ensureGrpcPrometheus() {
	grpcPromRegisterOnce.Do(func() {
		grpc_prometheus.EnableHandlingTimeHistogram()
	})
}

// ---- helpers ---------------------------------------------------------

// wrappedServerStream lets stream interceptors override ss.Context().
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context { return w.ctx }

// lookupSubappLocked scans portMap for an entry whose value matches port
// and returns the subapp prefix (before first '.'). Caller MUST already
// hold m.mu — used by GrpcServerOn / HttpRouterOn to avoid re-entering
// the lock via m.subappForListener (which itself locks).
func lookupSubappLocked(portMap map[string]int, port int) string {
	for key, p := range portMap {
		if p != port {
			continue
		}
		if idx := strings.Index(key, "."); idx >= 0 {
			return key[:idx]
		}
		return key
	}
	return ""
}
