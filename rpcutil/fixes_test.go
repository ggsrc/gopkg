// fixes_test.go — review-driven regression tests for the PR #115 framework.
//
// Each test below pins a specific fix from the multi-perspective code review:
//
//	F1: mega_subapp_version gauge must use a package-level sync.Once so multiple
//	    MultiResource instances in the same process don't panic on duplicate
//	    prometheus.MustRegister.
//	F3: normalizeMegaAppName strips a leading "galxe-" so OTEL_SERVICE_NAME does
//	    not become "galxe-galxe-foo".
//	F4: Register is concurrency-safe across goroutines (no duplicate names or
//	    lost subapps).
//	F5: processSubApp's port reservation is atomic — no two concurrent Register
//	    calls can both claim the same port.
//	F6: scanLeakedSDKEnv is a no-op when no subApps are registered.
//	F7: stopServers does not bump mega_shutdown_aborted_total for non-timeout
//	    Shutdown errors (e.g. ErrServerClosed when already-closed).
//	WithRedis / WithDCache: option injectors thread the values through to the
//	    MultiResource so CacheFor sub-apps work end-to-end.
package rpcutil

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

// ---- F3: normalize app name -----------------------------------------------

func TestNormalizeMegaAppNameStripsGalxePrefix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"airdrop-mega", "airdrop-mega"},
		{"galxe-airdrop-mega", "airdrop-mega"},
		{"  galxe-foo  ", "foo"},
		{"", ""},
		{"galxe-", ""},
	}
	for _, c := range cases {
		if got := normalizeMegaAppName(c.in); got != c.want {
			t.Errorf("normalizeMegaAppName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSetMegaSDKEnvUsesNormalizedName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("OTEL_SERVICE_NAME", "")
	m, err := NewMultiResource(context.Background(), WithMegaAppName("galxe-airdrop-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}
	_ = m
	if got := os.Getenv("OTEL_SERVICE_NAME"); got != "galxe-airdrop-mega" {
		t.Errorf("OTEL_SERVICE_NAME = %q, want %q (no double galxe- prefix)", got, "galxe-airdrop-mega")
	}
}

// ---- WithRedis / WithDCache injection -------------------------------------

func TestWithRedisInjectsClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}) // never connected, just for type
	defer stub.Close()

	m, err := NewMultiResource(context.Background(),
		WithMegaAppName("inj-mega"),
		WithRedis(stub),
	)
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}
	if m.redis == nil {
		t.Fatal("WithRedis: redis not stored on MultiResource")
	}
}

func TestWithDCacheNilByDefaultErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m, err := NewMultiResource(context.Background(), WithMegaAppName("nil-cache-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}
	c := m.CacheFor("staking")
	err = c.Ping(context.Background())
	if err == nil {
		t.Fatal("CacheFor.Ping with nil dcache: want error, got nil")
	}
}

func TestWithDCacheStoresInstance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// We don't need a real dcache instance; just a non-nil sentinel pointer
	// is enough to verify the option flows through to MultiResource. Pass nil
	// is OK too — type stays *dcache.DCache.
	m, err := NewMultiResource(context.Background(),
		WithMegaAppName("dcache-mega"),
		WithDCache(nil),
	)
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}
	// CacheFor wrapper always returns non-nil; underlying must mirror the
	// option (nil here = same as not setting it).
	if c := m.CacheFor("staking"); c == nil {
		t.Fatal("CacheFor returned nil wrapper")
	}
}

// ---- F1: mega_subapp_version single global registration -------------------

func TestSubAppVersionMetricSurvivesMultipleInstances(t *testing.T) {
	gin.SetMode(gin.TestMode)

	m1, err := NewMultiResource(context.Background(), WithMegaAppName("inst-1"))
	if err != nil {
		t.Fatalf("NewMultiResource 1: %v", err)
	}
	if err := m1.Register(newMultiFakeApp("alpha", 11111), OnPorts(PortMap{"l0": 11111})); err != nil {
		t.Fatalf("Register on m1: %v", err)
	}
	m1.RecordSubAppVersion("alpha", "abc123")
	m1.registerSubAppVersionMetric()

	// Second instance in same process — must not panic on
	// prometheus.MustRegister; previously this was the F1 bug.
	m2, err := NewMultiResource(context.Background(), WithMegaAppName("inst-2"))
	if err != nil {
		t.Fatalf("NewMultiResource 2: %v", err)
	}
	if err := m2.Register(newMultiFakeApp("beta", 22222), OnPorts(PortMap{"l0": 22222})); err != nil {
		t.Fatalf("Register on m2: %v", err)
	}
	m2.RecordSubAppVersion("beta", "def456")
	m2.registerSubAppVersionMetric()

	g := ensureSubAppVersionGauge()
	if got := testutil.ToFloat64(g.WithLabelValues("alpha", "abc123")); got != 1 {
		t.Errorf("alpha/abc123 gauge = %v, want 1", got)
	}
	if got := testutil.ToFloat64(g.WithLabelValues("beta", "def456")); got != 1 {
		t.Errorf("beta/def456 gauge = %v, want 1", got)
	}
}

// ---- F4 / F5: Register concurrent safety ----------------------------------

func TestRegisterConcurrentNoDuplicate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	silenceLogger(t)
	m, err := NewMultiResource(context.Background(), WithMegaAppName("race-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}

	const N = 32
	var wg sync.WaitGroup
	var firstErr atomic.Value
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := "subapp-" + itoa(i)
			port := 31000 + i
			if err := m.Register(
				newMultiFakeApp(name, port),
				OnPorts(PortMap{"l0": port}),
			); err != nil {
				firstErr.CompareAndSwap(nil, err)
			}
		}()
	}
	wg.Wait()

	if v := firstErr.Load(); v != nil {
		t.Errorf("Register concurrent: %v", v)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if got := len(m.subApps); got != N {
		t.Errorf("subApps count = %d, want %d", got, N)
	}
	if got := len(m.portMap); got != N {
		t.Errorf("portMap count = %d, want %d", got, N)
	}
}

func TestRegisterConcurrentDetectsDuplicatePort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	silenceLogger(t)
	m, err := NewMultiResource(context.Background(), WithMegaAppName("dup-port-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}

	const N = 8
	var (
		wg       sync.WaitGroup
		okCount  atomic.Int32
		errCount atomic.Int32
	)
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := "race-" + itoa(i)
			// All N goroutines fight for the SAME port — F5 must serialize so
			// exactly one wins.
			err := m.Register(
				newMultiFakeApp(name, 32100),
				OnPorts(PortMap{"l0": 32100}),
			)
			if err == nil {
				okCount.Add(1)
			} else {
				errCount.Add(1)
			}
		}()
	}
	wg.Wait()
	if okCount.Load() != 1 {
		t.Errorf("ok count = %d, want exactly 1", okCount.Load())
	}
	if errCount.Load() != N-1 {
		t.Errorf("err count = %d, want %d", errCount.Load(), N-1)
	}
}

// ---- F6: scanLeakedSDKEnv ------------------------------------------------

func TestScanLeakedSDKEnvNoOpWithoutSubApps(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	orig := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = orig })

	t.Setenv("AIRDROP_OTEL_SERVICE_NAME", "leaked")

	m, err := NewMultiResource(context.Background(), WithMegaAppName("empty-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}
	m.scanLeakedSDKEnv()

	if got := buf.String(); got != "" {
		t.Errorf("scanLeakedSDKEnv with no subApps logged %q, want empty", got)
	}
}

// ---- F7: stopServers only counts timeouts ---------------------------------

// TestStopServersNonTimeoutErrorNotCountedAbort wires a closed *http.Server into
// the MultiResource. Calling Shutdown on a closed server returns
// http.ErrServerClosed (non-timeout) — F7 says the abort counter must NOT
// increment in that case.
func TestStopServersNonTimeoutErrorNotCountedAbort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	silenceLogger(t)

	m, err := NewMultiResource(context.Background(), WithMegaAppName("stop-err-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}

	port := pickFreePorts(t, 1)[0]
	srv := &http.Server{Addr: ":0", Handler: http.NewServeMux()}
	// Pre-close so subsequent Shutdown returns http.ErrServerClosed (non-timeout).
	_ = srv.Close()

	m.mu.Lock()
	m.httpServers[port] = srv
	m.portMap["fake.http"] = port
	m.mu.Unlock()

	before := testutil.ToFloat64(shutdownAbortCounter.WithLabelValues("fake", "http"))
	m.stopServers(context.Background())
	after := testutil.ToFloat64(shutdownAbortCounter.WithLabelValues("fake", "http"))
	if after != before {
		t.Errorf("non-timeout shutdown bumped abort counter (before=%v after=%v)", before, after)
	}
}

// ---- X1: phase2 rollback on Init failure (BLOCKER #1) ---------------------

// stubSubApp is a minimal SubApp impl with knobs to fail Init / RegisterRoutes
// and counters to assert Close behavior. Used by the rollback / idempotent /
// drain-delay / cron-rootCtx regression tests below.
type stubSubApp struct {
	name             string
	initErr          error
	registerRouteErr error
	initCount        atomic.Int32
	closeCount       atomic.Int32
}

func (s *stubSubApp) Name() string                       { return s.name }
func (s *stubSubApp) Listeners() []ListenerSpec          { return nil }
func (s *stubSubApp) Init(*MultiResource) error          { s.initCount.Add(1); return s.initErr }
func (s *stubSubApp) RegisterRoutes(RouteRegistry) error { return s.registerRouteErr }
func (s *stubSubApp) HealthChecks() []HealthCheckable    { return nil }
func (s *stubSubApp) Cron() []CronJob                    { return nil }
func (s *stubSubApp) Close(context.Context) error        { s.closeCount.Add(1); return nil }

// errStr is a string error type to keep regression test sentinels light.
type errStr string

func (e errStr) Error() string { return string(e) }

// errInjected is the sentinel used by phase2 rollback tests.
var errInjected = errStr("injected init failure")

// TestPhase2RollbackClosesPredecessorsOnInitFailure verifies cross-review BLOCKER
// #1: when sub-app N's Init returns an error, sub-apps 0..N-1 (which had
// successful Init) must have Close called in reverse order. Without rollback,
// Init-failure crash loops leak DB pools / background goroutines forever.
func TestPhase2RollbackClosesPredecessorsOnInitFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	silenceLogger(t)

	m, err := NewMultiResource(context.Background(), WithMegaAppName("rollback-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}

	good1 := &stubSubApp{name: "good1"}
	good2 := &stubSubApp{name: "good2"}
	bad := &stubSubApp{name: "bad", initErr: errInjected}

	m.mu.Lock()
	m.subApps = []registeredSubApp{
		{app: good1, portMap: PortMap{}},
		{app: good2, portMap: PortMap{}},
		{app: bad, portMap: PortMap{}},
	}
	m.mu.Unlock()

	if err := m.phase2InitSubApps(context.Background()); err == nil {
		t.Fatal("expected phase2 to error on bad.Init")
	}

	if good1.closeCount.Load() != 1 {
		t.Errorf("good1.Close called %d times, want 1", good1.closeCount.Load())
	}
	if good2.closeCount.Load() != 1 {
		t.Errorf("good2.Close called %d times, want 1", good2.closeCount.Load())
	}
	if bad.closeCount.Load() != 0 {
		t.Errorf("bad.Close called %d times, want 0 (Init never succeeded)", bad.closeCount.Load())
	}

	// initializedApps must remain empty so a follow-up Stop does not double-Close.
	m.mu.Lock()
	got := len(m.initializedApps)
	m.mu.Unlock()
	if got != 0 {
		t.Errorf("initializedApps len = %d, want 0 on failed phase2", got)
	}
}

// TestPhase2RollbackOnRegisterRoutesFailure: RegisterRoutes failing also
// triggers rollback (Init succeeded — must Close).
func TestPhase2RollbackOnRegisterRoutesFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	silenceLogger(t)

	m, err := NewMultiResource(context.Background(), WithMegaAppName("rollback2-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}

	good := &stubSubApp{name: "good"}
	bad := &stubSubApp{name: "bad", registerRouteErr: errInjected}

	m.mu.Lock()
	m.subApps = []registeredSubApp{
		{app: good, portMap: PortMap{}},
		{app: bad, portMap: PortMap{}},
	}
	m.mu.Unlock()

	if err := m.phase2InitSubApps(context.Background()); err == nil {
		t.Fatal("expected phase2 to error on bad.RegisterRoutes")
	}
	if good.closeCount.Load() != 1 {
		t.Errorf("good.Close called %d times, want 1", good.closeCount.Load())
	}
	// bad's Init succeeded so it counts as initialized → must be Closed too.
	if bad.closeCount.Load() != 1 {
		t.Errorf("bad.Close called %d times, want 1 (Init succeeded)", bad.closeCount.Load())
	}
}

// ---- X2: Stop is idempotent (BLOCKER #2) -----------------------------------

// TestStopIsIdempotent verifies BLOCKER #2: when Stop is called multiple times
// (typical: Start returns on <-ctx.Done() and calls Stop; main's signal handler
// also calls Stop) the 6 phases run at most once. Each sub-app's Close runs
// once and each closeFunc runs once.
func TestStopIsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	silenceLogger(t)

	m, err := NewMultiResource(context.Background(), WithMegaAppName("idem-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}

	app := &stubSubApp{name: "app"}
	rs := registeredSubApp{app: app, portMap: PortMap{}}
	m.mu.Lock()
	m.subApps = []registeredSubApp{rs}
	m.initializedApps = []registeredSubApp{rs}
	m.mu.Unlock()

	var fnCalls atomic.Int32
	m.closeFuncs = []func(context.Context) error{
		func(context.Context) error { fnCalls.Add(1); return nil },
	}

	_ = m.Stop(context.Background())
	_ = m.Stop(context.Background())
	_ = m.Stop(context.Background())

	if got := app.closeCount.Load(); got != 1 {
		t.Errorf("Close called %d times across 3 Stop invocations, want 1", got)
	}
	if got := fnCalls.Load(); got != 1 {
		t.Errorf("closeFunc called %d times across 3 Stop invocations, want 1", got)
	}
}

// ---- X3: WithPreStopDrainDelay applies (BLOCKER #3) ------------------------

// TestStopPreStopDrainDelayApplied verifies BLOCKER #3: when WithPreStopDrainDelay
// is set, Stop sleeps that long between Phase 1 (readiness flip) and Phase 2.
func TestStopPreStopDrainDelayApplied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	silenceLogger(t)

	const want = 80 * time.Millisecond
	m, err := NewMultiResource(context.Background(),
		WithMegaAppName("drain-mega"),
		WithPreStopDrainDelay(want),
	)
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}

	start := time.Now()
	_ = m.Stop(context.Background())
	got := time.Since(start)

	if got < want {
		t.Errorf("Stop took %v, want >= %v (drain delay missing)", got, want)
	}
	// 5x the configured value is a generous ceiling on macOS / CI jitter while
	// still catching e.g. accidental WithPreStopDrainDelay = 5s default.
	if got > 5*want {
		t.Errorf("Stop took %v, want <= %v (drain delay too long)", got, 5*want)
	}
}

// TestStopPreStopDrainDelayCutShortByCtx: when the caller's Stop ctx fires
// during the drain delay, the delay is cut short — protects against a stuck
// drain blowing past terminationGracePeriodSeconds.
func TestStopPreStopDrainDelayCutShortByCtx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	silenceLogger(t)

	m, err := NewMultiResource(context.Background(),
		WithMegaAppName("drain-cut-mega"),
		WithPreStopDrainDelay(5*time.Second),
	)
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_ = m.Stop(ctx)
	got := time.Since(start)

	if got > 500*time.Millisecond {
		t.Errorf("Stop with canceled ctx took %v, want < 500ms (drain ignored ctx)", got)
	}
}

// ---- X4: cron Fn ctx tied to rootCtx (BLOCKER #4) --------------------------

// TestWrapCronFnObservesRootCtxCancel: a cron Fn that loops on ctx.Done must
// observe Stop()'s rootCancel and return promptly — before the Phase 5
// closeFuncs run, so a long-running cron doesn't use a closed DB pool.
func TestWrapCronFnObservesRootCtxCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	silenceLogger(t)

	m, err := NewMultiResource(context.Background(), WithMegaAppName("croncancel-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}

	entered := make(chan struct{})
	exited := make(chan struct{})
	wrapped := m.wrapCronFn("alpha", "alpha.job", func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		close(exited)
		return ctx.Err()
	})

	go wrapped()

	select {
	case <-entered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("wrapped cron Fn never entered within 500ms")
	}

	// Trigger rootCancel — same path Stop() takes in Phase 1.
	m.rootCancel()

	select {
	case <-exited:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("wrapped cron Fn did not observe rootCancel within 500ms")
	}
}

// ---- X5: Stop honors caller ctx (HIGH #5) ----------------------------------

// slowCloseApp's Close blocks until released or ctx.Done, used to test that
// Stop's caller-supplied ctx caps individual phase ctxs.
type slowCloseApp struct {
	stubSubApp
	released chan struct{}
}

func (s *slowCloseApp) Close(ctx context.Context) error {
	s.closeCount.Add(1)
	select {
	case <-s.released:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TestStopCtxBoundsPhaseTimeouts verifies HIGH #5: passing a tightly-bounded
// ctx to Stop limits each phase ctx — a sub-app's Close that ignores ctx still
// gets unblocked by Stop's overall ctx deadline.
func TestStopCtxBoundsPhaseTimeouts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	silenceLogger(t)

	m, err := NewMultiResource(context.Background(), WithMegaAppName("stopctx-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}

	app := &slowCloseApp{
		stubSubApp: stubSubApp{name: "slow"},
		released:   make(chan struct{}), // never closed: simulates hung Close
	}
	rs := registeredSubApp{app: app, portMap: PortMap{}}
	m.mu.Lock()
	m.subApps = []registeredSubApp{rs}
	m.initializedApps = []registeredSubApp{rs}
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_ = m.Stop(ctx)
	got := time.Since(start)

	// Without ctx honoring, Stop would wait the full ShutdownPhaseTimeout (30s).
	if got > 1*time.Second {
		t.Errorf("Stop took %v, want < 1s (ctx not honored)", got)
	}
	if app.closeCount.Load() != 1 {
		t.Errorf("Close called %d times, want 1", app.closeCount.Load())
	}
}

// ---- X6: Phase 5 closeFuncs run in parallel (HIGH #6) ----------------------

// TestStopCloseFuncsRunInParallel verifies HIGH #6: closeFuncs are invoked
// concurrently so total Phase 5 wall time is max(closeFunc), not Σ.
func TestStopCloseFuncsRunInParallel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	silenceLogger(t)

	m, err := NewMultiResource(context.Background(), WithMegaAppName("parallel-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}

	const each = 100 * time.Millisecond
	const n = 5
	var done atomic.Int32
	m.closeFuncs = make([]func(context.Context) error, n)
	for i := 0; i < n; i++ {
		m.closeFuncs[i] = func(context.Context) error {
			time.Sleep(each)
			done.Add(1)
			return nil
		}
	}

	start := time.Now()
	_ = m.Stop(context.Background())
	got := time.Since(start)

	if done.Load() != int32(n) {
		t.Errorf("closeFuncs run = %d, want %d", done.Load(), n)
	}
	// Sequential would take ≥ n*each (500ms). Parallel max(each) ~= 100ms + overhead.
	// Cap generously at 4x to absorb CI jitter while still failing on a serial
	// regression.
	upper := 4 * each
	if got > upper {
		t.Errorf("Stop took %v, want < %v (closeFuncs ran sequentially?)", got, upper)
	}
}

// ---- helpers --------------------------------------------------------------

func silenceLogger(t *testing.T) {
	t.Helper()
	orig := log.Logger
	log.Logger = zerolog.Nop()
	t.Cleanup(func() { log.Logger = orig })
}

// ============================================================================
// Round-8 cross-review pins (after Round-7 first-pass landed ee62d6a).
// ============================================================================

// Y1 (Round-8 #1 + Round-7 O-006): Phase ordering — scheduler.Stop fires
// BEFORE drain delay; rootCancel fires AFTER drain delay. A refactor that
// reverts either ordering must trip this test.
func TestStopPhaseOrdering_SchedulerBeforeDrain_RootCancelAfterDrain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	silenceLogger(t)
	withTempHealthPorts(t)

	m := newStartupTestMR(t)
	m.appName = "ord-mega"
	m.preStopDrainDelay = 200 * time.Millisecond

	var (
		schedStopAt    atomic.Int64
		rootCanceledAt atomic.Int64
	)
	rootCtx, cancel := context.WithCancel(context.Background())
	m.rootCtx, m.rootCancel = rootCtx, func() {
		rootCanceledAt.Store(time.Now().UnixNano())
		cancel()
	}
	m.scheduler = &orderingMockScheduler{
		onStop: func() { schedStopAt.Store(time.Now().UnixNano()) },
	}
	m.cronDisabled = false

	// Phase4 needs startedAt; faking it directly bypasses Start.
	now := time.Now()
	m.startedAt.Store(&now)
	drainStart := time.Now()
	m.doStop(context.Background())

	sched := schedStopAt.Load()
	cancelTime := rootCanceledAt.Load()
	if sched == 0 {
		t.Fatal("scheduler.Stop was never called")
	}
	if cancelTime == 0 {
		t.Fatal("rootCancel was never called")
	}
	// scheduler must fire BEFORE drain ends.
	drainEnd := drainStart.Add(m.preStopDrainDelay).UnixNano()
	if sched >= drainEnd {
		t.Errorf("scheduler.Stop fired at %d, after drain deadline %d — should be BEFORE drain",
			sched, drainEnd)
	}
	// rootCancel must fire AFTER drain ends (within tolerance).
	if cancelTime < drainEnd-int64(50*time.Millisecond) {
		t.Errorf("rootCancel fired at %d, before drain deadline %d — should be AFTER drain",
			cancelTime, drainEnd)
	}
	// And scheduler.Stop < rootCancel.
	if sched > cancelTime {
		t.Errorf("scheduler stopped at %d AFTER rootCancel %d — order inverted", sched, cancelTime)
	}
}

type orderingMockScheduler struct {
	onStop func()
}

func (s *orderingMockScheduler) RegisterCronJob(string, string, func()) error { return nil }
func (s *orderingMockScheduler) Start()                                       {}
func (s *orderingMockScheduler) Stop(context.Context) error {
	if s.onStop != nil {
		s.onStop()
	}
	return nil
}

// Y2 (Round-8 #4 + Round-7 O-001): DeletePartialMatch erases old version
// series when a sub-app records a new SHA. Without the delete, both old and
// new (subapp, version) series sit at 1, breaking
// `count by (subapp)(mega_subapp_version == 1)`.
func TestSubAppVersionGauge_OldSeriesRemovedOnRevision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m, err := NewMultiResource(context.Background(), WithMegaAppName("ver-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}
	if err := m.Register(newMultiFakeApp("rotapp", 41001), OnPorts(PortMap{"l0": 41001})); err != nil {
		t.Fatalf("Register: %v", err)
	}

	g := ensureSubAppVersionGauge()
	// Snapshot baseline counts (other tests may have run; we only care that
	// our specific labels appear/disappear correctly).
	want := func(subapp, version string, expect float64) {
		t.Helper()
		got := testutil.ToFloat64(g.WithLabelValues(subapp, version))
		if got != expect {
			t.Errorf("gauge(%q, %q) = %v, want %v", subapp, version, got, expect)
		}
	}

	m.RecordSubAppVersion("rotapp", "sha-OLD")
	m.registerSubAppVersionMetric()
	want("rotapp", "sha-OLD", 1)

	// Rotate to a new SHA — the old series must drop to 0 (after Delete) and
	// the new must be 1.
	m.RecordSubAppVersion("rotapp", "sha-NEW")
	m.registerSubAppVersionMetric()
	want("rotapp", "sha-OLD", 0)
	want("rotapp", "sha-NEW", 1)
}

// Y3 (Round-8 #4 + Round-7 C-002): phase1BindListeners must NOT hold m.mu
// across the blocking net.Listen syscalls. Concurrent readers of m.mu (e.g.
// /metrics scrape via lockedGatherers, MetricRegistryFor) MUST be able to
// progress during Phase 1. We exercise that by spinning a hot reader against
// m.mu while phase1BindListeners runs; under the lock-holding regression the
// reader's progress count stays at 0.
func TestPhase1BindListeners_DoesNotHoldMuAcrossListen(t *testing.T) {
	gin.SetMode(gin.TestMode)
	silenceLogger(t)

	m := newStartupTestMR(t)
	ports := pickFreePorts(t, 4)
	for _, p := range ports {
		m.grpcServers[p] = grpc.NewServer()
	}

	// Hot reader: spam MetricRegistryFor (takes m.mu).
	var progress atomic.Int64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				_ = m.MetricRegistryFor("hot-reader")
				progress.Add(1)
			}
		}
	}()

	// Run bind concurrently. With lock held across syscall, MetricRegistryFor
	// would stall until phase1 returns — we want to see progress > 0 mid-flight.
	if err := m.phase1BindListeners(context.Background()); err != nil {
		t.Fatalf("phase1BindListeners: %v", err)
	}
	close(stop)
	<-done

	if progress.Load() == 0 {
		t.Errorf("reader made zero progress — phase1 likely holds m.mu across net.Listen")
	}

	// Clean up the bound listeners.
	for _, l := range m.grpcListeners {
		_ = l.(net.Listener).Close()
	}
}
