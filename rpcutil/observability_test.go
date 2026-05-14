// observability_test.go — Wave 2 agent E tests for panic recover
// interceptors / subapp logger interceptors / observability.Go background
// goroutine / ensureGrpcPrometheus.
//
// White-box (package rpcutil) so we can poke private interceptors and
// metric vars.
package rpcutil

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gopkg_zerolog "github.com/ggsrc/gopkg/zerolog"
)

// ---- helpers -----------------------------------------------------------

// newObsTestMR returns a minimal MultiResource with maps eagerly initialised
// so listener factory methods can run. Named distinctly from other test
// helpers in the package to avoid cross-file collisions.
func newObsTestMR() *MultiResource {
	return &MultiResource{
		portMap:     map[string]int{},
		grpcServers: map[int]*grpc.Server{},
		httpRouters: map[int]*gin.Engine{},
	}
}

// shortenBackoff overrides goroutine backoff knobs for fast retry-loop
// tests and restores them via t.Cleanup.
func shortenBackoff(t *testing.T) {
	t.Helper()
	origInit, origMax := goroutineInitialBackoff, goroutineMaxBackoff
	goroutineInitialBackoff = 1 * time.Millisecond
	goroutineMaxBackoff = 4 * time.Millisecond
	t.Cleanup(func() {
		goroutineInitialBackoff = origInit
		goroutineMaxBackoff = origMax
	})
}

// fakeStream is a minimal grpc.ServerStream stub returning a fixed ctx.
type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeStream) Context() context.Context { return f.ctx }

// ---- A: observability.Go — fn returns nil → exits clean ---------------

func TestGoFnReturnsNilExitsCleanly(t *testing.T) {
	m := newObsTestMR()

	beforeFail := testutil.ToFloat64(goroutineFailCounter.WithLabelValues("staking", "noop"))
	beforePanic := testutil.ToFloat64(goroutinePanicCounter.WithLabelValues("staking", "noop"))

	done := make(chan struct{})
	m.Go("staking", "noop", func(ctx context.Context) error {
		close(done)
		return nil
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fn never ran")
	}
	// Allow goroutine to exit cleanly.
	time.Sleep(20 * time.Millisecond)

	afterFail := testutil.ToFloat64(goroutineFailCounter.WithLabelValues("staking", "noop"))
	afterPanic := testutil.ToFloat64(goroutinePanicCounter.WithLabelValues("staking", "noop"))
	if afterFail != beforeFail {
		t.Errorf("fail counter delta %v want 0", afterFail-beforeFail)
	}
	if afterPanic != beforePanic {
		t.Errorf("panic counter delta %v want 0", afterPanic-beforePanic)
	}
}

// ---- B: fn returns error twice then nil → failCounter +2, exits -------

func TestGoFnRetriesOnErrorThenExits(t *testing.T) {
	shortenBackoff(t)
	m := newObsTestMR()

	const subapp, name = "staking", "retry-then-ok"
	before := testutil.ToFloat64(goroutineFailCounter.WithLabelValues(subapp, name))

	var attempts atomic.Int32
	done := make(chan struct{})
	m.Go(subapp, name, func(ctx context.Context) error {
		n := attempts.Add(1)
		if n < 3 {
			return errors.New("transient")
		}
		close(done)
		return nil
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fn never reached success")
	}
	time.Sleep(20 * time.Millisecond)

	after := testutil.ToFloat64(goroutineFailCounter.WithLabelValues(subapp, name))
	if got := after - before; got != 2 {
		t.Errorf("fail counter delta %v, want 2", got)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

// ---- C: fn panics → panicCounter +1, retried --------------------------

func TestGoFnPanicsThenRecovers(t *testing.T) {
	shortenBackoff(t)
	m := newObsTestMR()

	const subapp, name = "staking", "panic-once"
	beforePanic := testutil.ToFloat64(goroutinePanicCounter.WithLabelValues(subapp, name))
	beforeFail := testutil.ToFloat64(goroutineFailCounter.WithLabelValues(subapp, name))

	var attempts atomic.Int32
	done := make(chan struct{})
	m.Go(subapp, name, func(ctx context.Context) error {
		n := attempts.Add(1)
		if n == 1 {
			panic("boom")
		}
		close(done)
		return nil
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("retry after panic never happened")
	}
	time.Sleep(20 * time.Millisecond)

	afterPanic := testutil.ToFloat64(goroutinePanicCounter.WithLabelValues(subapp, name))
	afterFail := testutil.ToFloat64(goroutineFailCounter.WithLabelValues(subapp, name))
	if got := afterPanic - beforePanic; got != 1 {
		t.Errorf("panic counter delta %v, want 1", got)
	}
	// runOnce converts panic to err, so failCounter also ticks once.
	if got := afterFail - beforeFail; got != 1 {
		t.Errorf("fail counter delta %v, want 1 (panic-as-error)", got)
	}
}

// ---- D: ctx canceled → goroutine exits cleanly ------------------------

func TestGoCtxCancelStopsRetryLoop(t *testing.T) {
	shortenBackoff(t)
	m := newObsTestMR()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Drive runGoroutineWithRetry directly so we control ctx (Go() uses
	// SubAppContext which is a Wave 3 stub returning context.Background()).
	const subapp, name = "staking", "ctx-cancel"
	exited := make(chan struct{})
	go func() {
		m.runGoroutineWithRetry(ctx, subapp, name, func(ctx context.Context) error {
			return errors.New("always fails to force retry loop")
		})
		close(exited)
	}()

	// Let it spin once, then cancel.
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("retry loop did not exit on ctx cancel")
	}
}

// ---- E: panicRecoverUnaryInterceptor recovers, sets Internal ----------

func TestPanicRecoverUnaryInterceptorRecovers(t *testing.T) {
	m := newObsTestMR()
	const subapp, method = "staking", "/test.Svc/Method"
	before := testutil.ToFloat64(panicCounter.WithLabelValues(subapp, "unary", method))

	intr := m.panicRecoverUnaryInterceptor(subapp)
	info := &grpc.UnaryServerInfo{FullMethod: method}
	resp, err := intr(context.Background(), nil, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		panic("kaboom")
	})

	if resp != nil {
		t.Errorf("resp want nil, got %v", resp)
	}
	if err == nil {
		t.Fatal("expected error after panic")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Errorf("want codes.Internal, got %v", err)
	}

	after := testutil.ToFloat64(panicCounter.WithLabelValues(subapp, "unary", method))
	if got := after - before; got != 1 {
		t.Errorf("panic counter delta %v want 1", got)
	}
}

// ---- F: passthrough when no panic -------------------------------------

func TestPanicRecoverUnaryInterceptorPassthrough(t *testing.T) {
	m := newObsTestMR()
	const subapp, method = "staking", "/test.Svc/OK"
	before := testutil.ToFloat64(panicCounter.WithLabelValues(subapp, "unary", method))

	intr := m.panicRecoverUnaryInterceptor(subapp)
	info := &grpc.UnaryServerInfo{FullMethod: method}
	resp, err := intr(context.Background(), "req", info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp != "ok" {
		t.Errorf("resp = %v want ok", resp)
	}
	after := testutil.ToFloat64(panicCounter.WithLabelValues(subapp, "unary", method))
	if after != before {
		t.Errorf("counter incremented on ok path: delta %v", after-before)
	}
}

// ---- G: stream interceptor recovers -----------------------------------

func TestPanicRecoverStreamInterceptorRecovers(t *testing.T) {
	m := newObsTestMR()
	const subapp, method = "staking", "/test.Svc/Stream"
	before := testutil.ToFloat64(panicCounter.WithLabelValues(subapp, "stream", method))

	intr := m.panicRecoverStreamInterceptor(subapp)
	info := &grpc.StreamServerInfo{FullMethod: method}
	ss := &fakeStream{ctx: context.Background()}
	err := intr(nil, ss, info, func(srv interface{}, ss grpc.ServerStream) error {
		panic("stream-boom")
	})

	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Errorf("want codes.Internal, got %v", err)
	}

	after := testutil.ToFloat64(panicCounter.WithLabelValues(subapp, "stream", method))
	if got := after - before; got != 1 {
		t.Errorf("counter delta %v want 1", got)
	}
}

// ---- H: HTTP middleware recovers → 500 --------------------------------

func TestPanicRecoverHttpMiddlewareRecovers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newObsTestMR()
	const subapp = "staking"

	eng := gin.New()
	eng.Use(m.panicRecoverHttpMiddleware(subapp))
	eng.GET("/boom", func(c *gin.Context) { panic("http-boom") })

	// FullPath() yields the route pattern, not the request URL.
	before := testutil.ToFloat64(panicCounter.WithLabelValues(subapp, "http", "/boom"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	eng.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status %d want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), "internal server error") {
		t.Errorf("body = %q, want 'internal server error'", w.Body.String())
	}
	after := testutil.ToFloat64(panicCounter.WithLabelValues(subapp, "http", "/boom"))
	if got := after - before; got != 1 {
		t.Errorf("counter delta %v want 1", got)
	}
}

// ---- I: subapp logger unary interceptor injects subapp ctx ------------

func TestSubappLoggerUnaryInterceptorInjectsSubApp(t *testing.T) {
	m := newObsTestMR()
	const subapp = "airdrop"

	intr := m.subappLoggerUnaryInterceptor(subapp)
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Svc/M"}

	var seenCtx context.Context
	_, err := intr(context.Background(), "req", info, func(ctx context.Context, req interface{}) (interface{}, error) {
		seenCtx = ctx
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	got, ok := gopkg_zerolog.SubAppFromCtx(seenCtx)
	if !ok || got != subapp {
		t.Errorf("subapp from ctx = (%q,%v), want (%q,true)", got, ok, subapp)
	}
}

// ---- I.2: stream interceptor injects subapp via wrappedServerStream ---

func TestSubappLoggerStreamInterceptorWrapsCtx(t *testing.T) {
	m := newObsTestMR()
	const subapp = "token"

	intr := m.subappLoggerStreamInterceptor(subapp)
	info := &grpc.StreamServerInfo{FullMethod: "/test.Svc/S"}
	ss := &fakeStream{ctx: context.Background()}

	var seen string
	var seenOk bool
	err := intr(nil, ss, info, func(srv interface{}, stream grpc.ServerStream) error {
		seen, seenOk = gopkg_zerolog.SubAppFromCtx(stream.Context())
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !seenOk || seen != subapp {
		t.Errorf("subapp from stream ctx = (%q,%v), want (%q,true)", seen, seenOk, subapp)
	}
}

// ---- J: HTTP middleware injects subapp into request ctx ---------------

func TestSubappLoggerHttpMiddlewareInjectsSubApp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newObsTestMR()
	const subapp = "campaign"

	eng := gin.New()
	eng.Use(m.subappLoggerHttpMiddleware(subapp))
	var seen string
	var seenOk bool
	eng.GET("/x", func(c *gin.Context) {
		seen, seenOk = gopkg_zerolog.SubAppFromCtx(c.Request.Context())
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	eng.ServeHTTP(w, req)

	if !seenOk || seen != subapp {
		t.Errorf("subapp from req ctx = (%q,%v), want (%q,true)", seen, seenOk, subapp)
	}
}

// ---- J.2: migrationMode adds legacy_app field (smoke via injectSubAppCtx)
// We can't read structured zerolog fields easily; verify code path doesn't
// panic and ctx still carries subapp.

func TestInjectSubAppCtxMigrationMode(t *testing.T) {
	m := newObsTestMR()
	m.migrationMode = true
	ctx := m.injectSubAppCtx(context.Background(), "airdrop")
	got, ok := gopkg_zerolog.SubAppFromCtx(ctx)
	if !ok || got != "airdrop" {
		t.Errorf("subapp from ctx = (%q,%v); want (airdrop,true)", got, ok)
	}
}

// ---- K: ensureGrpcPrometheus is idempotent ----------------------------

func TestEnsureGrpcPrometheusIdempotent(t *testing.T) {
	m := newObsTestMR()
	// First call may register, second call must not panic (sync.Once).
	m.ensureGrpcPrometheus()
	m.ensureGrpcPrometheus()
	// Also from a different MR instance.
	m2 := newObsTestMR()
	m2.ensureGrpcPrometheus()
}

// ---- L: integration smoke — GrpcServerOn / HttpRouterOn after wiring --

func TestGrpcServerOnReturnsNonNilAfterWiring(t *testing.T) {
	m := newObsTestMR()
	s1 := m.GrpcServerOn(19090)
	s2 := m.GrpcServerOn(19090)
	if s1 == nil || s2 == nil {
		t.Fatal("nil grpc server")
	}
	if s1 != s2 {
		t.Error("expected same *grpc.Server on repeat")
	}
}

func TestHttpRouterOnReturnsNonNilAfterWiring(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newObsTestMR()
	e1 := m.HttpRouterOn(13000)
	e2 := m.HttpRouterOn(13000)
	if e1 == nil || e2 == nil {
		t.Fatal("nil gin engine")
	}
	if e1 != e2 {
		t.Error("expected same *gin.Engine on repeat")
	}
}

// ---- L.2: HttpRouterOn produces engine that recovers panics -----------
//
// This test exercises the full wiring (panic-recover middleware actually
// installed by HttpRouterOn).

func TestHttpRouterOnWiresPanicRecover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newObsTestMR()
	e := m.HttpRouterOn(13001)
	e.GET("/boom", func(c *gin.Context) { panic("wired-boom") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	e.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d want 500 (panic recover middleware missing?)", w.Code)
	}
}

// ---- L.3: HttpRouterOn injects subapp ctx via wired middleware --------

func TestHttpRouterOnWiresSubAppCtx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newObsTestMR()
	// Pre-populate portMap so subappForListener can resolve.
	m.portMap["airdrop.http-rest"] = 13002

	e := m.HttpRouterOn(13002)
	var seen string
	var seenOk bool
	e.GET("/y", func(c *gin.Context) {
		seen, seenOk = gopkg_zerolog.SubAppFromCtx(c.Request.Context())
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/y", nil)
	e.ServeHTTP(w, req)

	if !seenOk || seen != "airdrop" {
		t.Errorf("subapp from wired engine ctx = (%q,%v), want (airdrop,true)", seen, seenOk)
	}
}

// ---- lookupSubappLocked unit ------------------------------------------

func TestLookupSubappLocked(t *testing.T) {
	pm := map[string]int{
		"airdrop.grpc":       9090,
		"airdrop.http-rest":  3000,
		"token.grpc":         9091,
		"plain":              7777, // no dot
	}
	if got := lookupSubappLocked(pm, 9090); got != "airdrop" {
		t.Errorf("port 9090 -> %q, want airdrop", got)
	}
	if got := lookupSubappLocked(pm, 9091); got != "token" {
		t.Errorf("port 9091 -> %q, want token", got)
	}
	if got := lookupSubappLocked(pm, 7777); got != "plain" {
		t.Errorf("port 7777 -> %q, want plain (no dot fallback)", got)
	}
	if got := lookupSubappLocked(pm, 12345); got != "" {
		t.Errorf("unknown port -> %q, want empty", got)
	}
}

// ---- empty-subapp interceptor doesn't panic (Wave 1 regression) -------

func TestInterceptorsHandleEmptySubapp(t *testing.T) {
	m := newObsTestMR()

	// Unary panic recover with empty subapp.
	intr := m.panicRecoverUnaryInterceptor("")
	info := &grpc.UnaryServerInfo{FullMethod: "/Svc/M"}
	_, err := intr(context.Background(), nil, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})
	if err != nil {
		t.Errorf("unexpected err with empty subapp: %v", err)
	}

	// HTTP middleware injects ctx with empty subapp without panic.
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	eng.Use(m.subappLoggerHttpMiddleware(""))
	eng.GET("/z", func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/z", nil)
	eng.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d want 200", w.Code)
	}
}

// ---- metric placeholders: pass-through invariants ---------------------

func TestMetricUnaryInterceptorPassthrough(t *testing.T) {
	m := newObsTestMR()
	intr := m.metricUnaryInterceptor("staking")
	info := &grpc.UnaryServerInfo{FullMethod: "/M"}
	resp, err := intr(context.Background(), "req", info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})
	if err != nil || resp != "ok" {
		t.Errorf("metric unary passthrough broken: resp=%v err=%v", resp, err)
	}
}

func TestMetricStreamInterceptorPassthrough(t *testing.T) {
	m := newObsTestMR()
	intr := m.metricStreamInterceptor("staking")
	info := &grpc.StreamServerInfo{FullMethod: "/M"}
	ss := &fakeStream{ctx: context.Background()}
	called := false
	err := intr(nil, ss, info, func(srv interface{}, s grpc.ServerStream) error {
		called = true
		return nil
	})
	if err != nil || !called {
		t.Errorf("metric stream passthrough broken: called=%v err=%v", called, err)
	}
}

func TestMetricHttpMiddlewarePassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newObsTestMR()
	eng := gin.New()
	eng.Use(m.metricHttpMiddleware("staking"))
	eng.GET("/p", func(c *gin.Context) { c.Status(http.StatusTeapot) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	eng.ServeHTTP(w, req)
	if w.Code != http.StatusTeapot {
		t.Errorf("status %d want 418", w.Code)
	}
}

// ---- concurrency: many Go() goroutines exit cleanly -------------------

func TestGoManyGoroutinesExitCleanly(t *testing.T) {
	m := newObsTestMR()
	var wg sync.WaitGroup
	const n = 20
	for i := 0; i < n; i++ {
		wg.Add(1)
		m.Go("staking", "many", func(ctx context.Context) error {
			defer wg.Done()
			return nil
		})
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("not all goroutines exited")
	}
}
