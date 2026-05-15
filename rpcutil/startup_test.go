// startup_test.go — Wave 4 agent H tests for Start/Stop 4+6 phase orchestration.
// White-box (package rpcutil) so we can poke private fields/helpers.
package rpcutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

// ---- helpers ---------------------------------------------------------------

// startupTestApp implements SubApp with overridable hooks.
type startupTestApp struct {
	name        string
	listeners   []ListenerSpec
	health      []HealthCheckable
	cron        []CronJob
	initErr     error
	initCount   atomic.Int32
	registerErr error
	regCount    atomic.Int32
	closeErr    error
	closeCount  atomic.Int32
	closeDelay  time.Duration
}

func (a *startupTestApp) Name() string              { return a.name }
func (a *startupTestApp) Listeners() []ListenerSpec { return a.listeners }
func (a *startupTestApp) Init(deps *MultiResource) error {
	a.initCount.Add(1)
	return a.initErr
}
func (a *startupTestApp) RegisterRoutes(reg RouteRegistry) error {
	a.regCount.Add(1)
	return a.registerErr
}
func (a *startupTestApp) HealthChecks() []HealthCheckable { return a.health }
func (a *startupTestApp) Cron() []CronJob                 { return a.cron }
func (a *startupTestApp) Close(ctx context.Context) error {
	a.closeCount.Add(1)
	if a.closeDelay > 0 {
		select {
		case <-time.After(a.closeDelay):
		case <-ctx.Done():
		}
	}
	return a.closeErr
}

// newStartupTestMR builds a MultiResource with the bare minimum maps initialized
// (mirrors what NewMultiResource would do, but bypasses gocron + sdk env for
// quick unit tests).
func newStartupTestMR(t *testing.T) *MultiResource {
	t.Helper()
	gin.SetMode(gin.TestMode)
	logger := zerolog.Nop()
	sa := &atomic.Pointer[time.Time]{}
	return &MultiResource{
		appName:       "test-mega",
		rootLogger:    &logger,
		migrationMode: false,
		portMap:       map[string]int{},
		grpcServers:   map[int]*grpc.Server{},
		grpcListeners: map[int]any{},
		httpRouters:   map[int]*gin.Engine{},
		httpListeners: map[int]any{},
		httpServers:   map[int]any{},
		dbPools:       nil,
		dbConfigs:     nil,
		caches:        nil,
		registries:    map[string]*prometheus.Registry{},
		gatherers:     prometheus.Gatherers{prometheus.DefaultGatherer},
		cronJobs:      map[string]bool{},
		startedAt:     sa,
	}
}

// pickFreePorts grabs N ephemeral TCP ports by binding port 0 then closing
// immediately. Race-prone but acceptable for tests that immediately re-bind.
func pickFreePorts(t *testing.T, n int) []int {
	t.Helper()
	ports := make([]int, 0, n)
	closers := make([]net.Listener, 0, n)
	for i := 0; i < n; i++ {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("pick free port: %v", err)
		}
		ports = append(ports, lis.Addr().(*net.TCPAddr).Port)
		closers = append(closers, lis)
	}
	for _, c := range closers {
		_ = c.Close()
	}
	return ports
}

// withTempHealthPorts overrides healthListenPort / metricListenPort to ephemeral
// values to avoid colliding with anything on 8080/4014.
func withTempHealthPorts(t *testing.T) {
	t.Helper()
	ports := pickFreePorts(t, 2)
	origH, origM := healthListenPort, metricListenPort
	healthListenPort = ports[0]
	metricListenPort = ports[1]
	t.Cleanup(func() {
		healthListenPort = origH
		metricListenPort = origM
	})
}

// dialTCP returns nil if a TCP connection to port can be established quickly.
func dialTCP(port int, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), timeout)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

// ---- A: phase1BindListeners empty ------------------------------------------

func TestPhase1BindListenersEmpty(t *testing.T) {
	m := newStartupTestMR(t)
	if err := m.phase1BindListeners(context.Background()); err != nil {
		t.Fatalf("phase1 with empty maps: %v", err)
	}
	if len(m.grpcListeners) != 0 || len(m.httpListeners) != 0 {
		t.Errorf("no listeners should be created, got grpc=%d http=%d",
			len(m.grpcListeners), len(m.httpListeners))
	}
}

// ---- B: phase1BindListeners failure path -----------------------------------

func TestPhase1BindListenersFailsOnTakenPort(t *testing.T) {
	// Bind on the same address spec phase1 uses (":port" wildcard) so the
	// collision is detected even on macOS where 127.0.0.1:p + 0.0.0.0:p can
	// coexist with REUSEADDR semantics.
	blocker, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("blocker listen: %v", err)
	}
	defer blocker.Close()
	port := blocker.Addr().(*net.TCPAddr).Port

	m := newStartupTestMR(t)
	m.grpcServers[port] = grpc.NewServer()

	err = m.phase1BindListeners(context.Background())
	if err == nil {
		t.Fatalf("expected listen error when port is taken")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf(":%d", port)) {
		t.Errorf("error should mention port %d, got: %v", port, err)
	}
}

// ---- C: phase2InitSubApps Init error ---------------------------------------

func TestPhase2InitSubAppsInitError(t *testing.T) {
	m := newStartupTestMR(t)
	bad := &startupTestApp{name: "bad", initErr: errors.New("boom")}
	good := &startupTestApp{name: "good"}
	m.subApps = []registeredSubApp{
		{app: bad, portMap: PortMap{}},
		{app: good, portMap: PortMap{}},
	}

	err := m.phase2InitSubApps(context.Background())
	if err == nil {
		t.Fatal("expected init error")
	}
	if !strings.Contains(err.Error(), "subapp=bad Init") {
		t.Errorf("error should wrap subapp name, got: %v", err)
	}
	if good.initCount.Load() != 0 {
		t.Errorf("subsequent sub-app must not be initialized, got count=%d",
			good.initCount.Load())
	}
}

// ---- D: phase2InitSubApps RegisterRoutes error -----------------------------

func TestPhase2InitSubAppsRegisterRoutesError(t *testing.T) {
	m := newStartupTestMR(t)
	bad := &startupTestApp{name: "bad", registerErr: errors.New("route boom")}
	m.subApps = []registeredSubApp{{app: bad, portMap: PortMap{}}}

	err := m.phase2InitSubApps(context.Background())
	if err == nil {
		t.Fatal("expected register error")
	}
	if !strings.Contains(err.Error(), "subapp=bad RegisterRoutes") {
		t.Errorf("error should wrap subapp name, got: %v", err)
	}
	if bad.initCount.Load() != 1 {
		t.Errorf("Init must have been called once, got %d", bad.initCount.Load())
	}
}

// ---- E: phase2InitSubApps registers health checks --------------------------

func TestPhase2InitSubAppsRegistersHealthChecks(t *testing.T) {
	m := newStartupTestMR(t)
	hc := HealthCheckable{
		Name:        "dep1",
		Criticality: Critical,
		Check:       func(ctx context.Context) error { return nil },
	}
	app := &startupTestApp{name: "alpha", health: []HealthCheckable{hc}}
	m.subApps = []registeredSubApp{{app: app, portMap: PortMap{}}}

	if err := m.phase2InitSubApps(context.Background()); err != nil {
		t.Fatalf("phase2: %v", err)
	}
	if len(m.healthChecks) != 1 {
		t.Fatalf("expected 1 healthCheck registered, got %d", len(m.healthChecks))
	}
	if m.healthChecks[0].subapp != "alpha" || m.healthChecks[0].hc.Name != "dep1" {
		t.Errorf("unexpected healthCheck: %+v", m.healthChecks[0])
	}
}

// ---- F: phase3 with cron disabled skips scheduler.Start --------------------

func TestPhase3RegisterCronDisabled(t *testing.T) {
	sched := newMockScheduler()
	m := newStartupTestMR(t)
	m.scheduler = sched
	m.cronDisabled = true

	app := &startupTestApp{
		name: "alpha",
		cron: []CronJob{{Name: "j", Schedule: "* * * * *", Fn: func(ctx context.Context) error { return nil }}},
	}
	m.subApps = []registeredSubApp{{app: app, portMap: PortMap{}}}

	if err := m.phase3RegisterCron(context.Background()); err != nil {
		t.Fatalf("phase3: %v", err)
	}
	if sched.started {
		t.Error("scheduler.Start must NOT be called when cronDisabled")
	}
	if sched.jobCount() != 0 {
		t.Errorf("no jobs should be registered when disabled, got %d", sched.jobCount())
	}
}

// ---- G: phase3 with cron enabled starts scheduler --------------------------

func TestPhase3RegisterCronEnabled(t *testing.T) {
	sched := newMockScheduler()
	m := newStartupTestMR(t)
	m.scheduler = sched
	m.cronDisabled = false

	app := &startupTestApp{
		name: "alpha",
		cron: []CronJob{{Name: "j", Schedule: "* * * * *", Fn: func(ctx context.Context) error { return nil }}},
	}
	m.subApps = []registeredSubApp{{app: app, portMap: PortMap{}}}

	if err := m.phase3RegisterCron(context.Background()); err != nil {
		t.Fatalf("phase3: %v", err)
	}
	if !sched.started {
		t.Error("scheduler.Start expected when cronDisabled=false")
	}
	if sched.jobCount() != 1 {
		t.Errorf("expected 1 cron job registered, got %d", sched.jobCount())
	}
}

// ---- H: full Start/Stop happy path -----------------------------------------

func TestStartStopHappyPath(t *testing.T) {
	withTempHealthPorts(t)
	sched := newMockScheduler()
	m := newStartupTestMR(t)
	m.scheduler = sched
	m.cronDisabled = false

	// subapp version metric is now package-level sync.Once — no per-instance
	// pre-fire needed (F1 fix). Test relies on single-Once behavior.

	ports := pickFreePorts(t, 1)
	grpcPort := ports[0]

	app := &startupTestApp{
		name: "alpha",
		listeners: []ListenerSpec{
			{Name: "grpc", Protocol: "grpc", PreferredPort: grpcPort},
		},
	}
	if err := m.processSubApp(app, OnPorts(PortMap{"grpc": grpcPort})); err != nil {
		t.Fatalf("processSubApp: %v", err)
	}
	m.subApps = append(m.subApps, registeredSubApp{
		app:     app,
		portMap: PortMap{"grpc": grpcPort},
	})

	ctx, cancel := context.WithCancel(context.Background())
	startDone := make(chan error, 1)
	go func() {
		startDone <- m.Start(ctx)
	}()

	deadline := time.Now().Add(3 * time.Second)
	readyURL := fmt.Sprintf("http://127.0.0.1:%d/health/ready", healthListenPort)
	for {
		if time.Now().After(deadline) {
			t.Fatal("/health/ready never returned 200 within deadline")
		}
		resp, err := http.Get(readyURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := dialTCP(grpcPort, 500*time.Millisecond); err != nil {
		t.Errorf("dial grpc port %d: %v", grpcPort, err)
	}

	cancel()
	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5s of ctx cancel")
	}

	if !sched.started {
		t.Error("scheduler should have been started")
	}
	if !sched.stopped {
		t.Error("scheduler should have been stopped")
	}
	if app.closeCount.Load() != 1 {
		t.Errorf("expected SubApp.Close once, got %d", app.closeCount.Load())
	}
}

// ---- I: Stop clears startedAt → readiness returns 503 ----------------------

func TestStopClearsStartedAt(t *testing.T) {
	m := newStartupTestMR(t)
	now := time.Now()
	m.startedAt.Store(&now)
	if m.startedAt.Load() == nil {
		t.Fatal("preset startedAt failed")
	}

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if m.startedAt.Load() != nil {
		t.Error("Stop must clear startedAt to flip readiness to 503")
	}

	req, _ := http.NewRequest(http.MethodGet, "/health/ready", nil)
	rw := &responseRecorder{header: http.Header{}}
	m.readinessHandler(rw, req)
	if rw.status != http.StatusServiceUnavailable {
		t.Errorf("expected 503 after Stop, got %d", rw.status)
	}
}

// responseRecorder is a minimal http.ResponseWriter for handler unit tests.
type responseRecorder struct {
	header http.Header
	status int
	body   strings.Builder
}

func (r *responseRecorder) Header() http.Header         { return r.header }
func (r *responseRecorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *responseRecorder) WriteHeader(s int)           { r.status = s }

// ---- J: stopServers empty / repeated --------------------------------------
//
// SKIP NOTE (spec case J): grpc graceful-timeout abort path. Real coverage
// requires (a) hijacking GracefulStopTimeout (const, untouchable from tests
// without modifying production code outside Wave 4 ownership) OR (b) a live
// streaming RPC pinned open for >30s (slow + flaky on CI). The equivalent
// abort-counter line for the close-error path is covered by case M
// (TestStopSubAppCloseErrorBumpsAbort). We instead assert stopServers handles
// the empty-state and double-call cases without panicking.
func TestStopServersEmptyAndRepeated(t *testing.T) {
	m := newStartupTestMR(t)
	// Empty maps → no-op.
	m.stopServers(context.Background())
	// Repeated call must remain safe (Stop idempotency precondition).
	m.stopServers(context.Background())
}

// ---- K: Stop is idempotent / safe to call twice ----------------------------

func TestStopIdempotent(t *testing.T) {
	m := newStartupTestMR(t)
	now := time.Now()
	m.startedAt.Store(&now)

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop #1: %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop #2: %v", err)
	}
}

// ---- L: routeRegistry GRPC / HTTP lookups ----------------------------------

func TestRouteRegistryLookups(t *testing.T) {
	m := newStartupTestMR(t)
	port := pickFreePorts(t, 1)[0]
	m.GrpcServerOn(port)

	httpPort := pickFreePorts(t, 1)[0]
	m.HttpRouterOn(httpPort)

	r := newRouteRegistry(m, "alpha", PortMap{"grpc": port, "http": httpPort})

	if r.GRPC("grpc") == nil {
		t.Error("expected non-nil *grpc.Server for known listener")
	}
	if r.GRPC("missing") != nil {
		t.Error("expected nil for unknown listener")
	}
	if r.HTTP("http") == nil {
		t.Error("expected non-nil *gin.Engine for known listener")
	}
	if r.HTTP("missing") != nil {
		t.Error("expected nil for unknown listener")
	}
}

// ---- M: subAppCloseError increments abort counter --------------------------

func TestStopSubAppCloseErrorBumpsAbort(t *testing.T) {
	m := newStartupTestMR(t)
	now := time.Now()
	m.startedAt.Store(&now)
	bad := &startupTestApp{name: "boomapp", closeErr: errors.New("nope")}
	rs := registeredSubApp{app: bad, portMap: PortMap{}}
	m.subApps = []registeredSubApp{rs}
	// Stop's Phase 4 only Closes initialized sub-apps (BLOCKER #1 fix).
	// Tests that bypass phase2 must seed initializedApps explicitly.
	m.initializedApps = []registeredSubApp{rs}

	before := testutil.ToFloat64(shutdownAbortCounter.WithLabelValues("boomapp", "close"))
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	after := testutil.ToFloat64(shutdownAbortCounter.WithLabelValues("boomapp", "close"))
	if after <= before {
		t.Errorf("expected close abort counter to increase: before=%v after=%v",
			before, after)
	}
	if bad.closeCount.Load() != 1 {
		t.Errorf("expected Close called once, got %d", bad.closeCount.Load())
	}
}

// ---- N: closeFuncs are invoked in Stop Phase 5 -----------------------------

func TestStopRunsCloseFuncs(t *testing.T) {
	m := newStartupTestMR(t)
	var called atomic.Int32
	m.closeFuncs = []func(context.Context) error{
		func(ctx context.Context) error { called.Add(1); return nil },
		func(ctx context.Context) error { called.Add(1); return errors.New("ignored") },
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if called.Load() != 2 {
		t.Errorf("expected both closeFuncs to run, got %d", called.Load())
	}
}

// ---- O: phase4 errors when listener not bound ------------------------------

func TestPhase4ErrorsWhenListenerMissing(t *testing.T) {
	m := newStartupTestMR(t)
	port := 7777
	m.grpcServers[port] = grpc.NewServer()
	// (no grpcListeners[port])

	err := m.phase4OpenHealth(context.Background())
	if err == nil {
		t.Fatal("expected error when no listener bound for grpc port")
	}
	if !strings.Contains(err.Error(), "no listener bound") {
		t.Errorf("error should mention missing listener, got: %v", err)
	}
}

// ---- P: subAppByPort wires shutdown labels --------------------------------

func TestSubAppByPortAfterRegister(t *testing.T) {
	m := newStartupTestMR(t)
	app := &startupTestApp{
		name: "labeled",
		listeners: []ListenerSpec{
			{Name: "grpc", Protocol: "grpc", PreferredPort: 60123},
		},
	}
	if err := m.processSubApp(app, OnPorts(PortMap{"grpc": 60123})); err != nil {
		t.Fatalf("processSubApp: %v", err)
	}
	if got := m.subAppByPort(60123); got != "labeled" {
		t.Errorf("subAppByPort(60123) = %q, want %q", got, "labeled")
	}
}

// Lint guard: ensure sync.WaitGroup is referenced (used in production code).
var _ = sync.WaitGroup{}

// ---- Q: phase1 binds http listeners too -----------------------------------

func TestPhase1BindsHttpListener(t *testing.T) {
	m := newStartupTestMR(t)
	httpPort := pickFreePorts(t, 1)[0]
	m.HttpRouterOn(httpPort) // registers gin engine for that port

	if err := m.phase1BindListeners(context.Background()); err != nil {
		t.Fatalf("phase1: %v", err)
	}
	lis, ok := m.httpListeners[httpPort].(net.Listener)
	if !ok || lis == nil {
		t.Fatalf("expected http listener bound on port %d", httpPort)
	}
	_ = lis.Close()
}

// ---- R: phase1 http listener fail surfaces error --------------------------

func TestPhase1HttpListenerFailure(t *testing.T) {
	blocker, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("blocker listen: %v", err)
	}
	defer blocker.Close()
	port := blocker.Addr().(*net.TCPAddr).Port

	m := newStartupTestMR(t)
	m.HttpRouterOn(port)

	err = m.phase1BindListeners(context.Background())
	if err == nil {
		t.Fatal("expected listen error for taken http port")
	}
}

// ---- S: phase4 errors when http listener missing --------------------------

func TestPhase4ErrorsWhenHttpListenerMissing(t *testing.T) {
	m := newStartupTestMR(t)
	port := 7778
	m.HttpRouterOn(port)
	// no httpListeners[port]
	err := m.phase4OpenHealth(context.Background())
	if err == nil {
		t.Fatal("expected error when no listener bound for http port")
	}
	if !strings.Contains(err.Error(), "no listener bound") {
		t.Errorf("error should mention missing listener, got: %v", err)
	}
}

// ---- T: stopServers with live grpc + http servers exercises goroutine fork

func TestStopServersWithLiveServersExitsCleanly(t *testing.T) {
	m := newStartupTestMR(t)

	// Spin up a real grpc.Server on an ephemeral port.
	grpcPort := pickFreePorts(t, 1)[0]
	gs := grpc.NewServer()
	gLis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		t.Fatalf("listen grpc: %v", err)
	}
	go func() { _ = gs.Serve(gLis) }()
	m.grpcServers[grpcPort] = gs

	// Spin up a real *http.Server on an ephemeral port.
	httpPort := pickFreePorts(t, 1)[0]
	hLis, err := net.Listen("tcp", fmt.Sprintf(":%d", httpPort))
	if err != nil {
		t.Fatalf("listen http: %v", err)
	}
	hs := &http.Server{Handler: http.NewServeMux()}
	go func() { _ = hs.Serve(hLis) }()
	m.httpServers[httpPort] = hs

	// Give the servers a moment to enter Accept loops.
	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		m.stopServers(context.Background())
		close(done)
	}()
	select {
	case <-done:
		// ok
	case <-time.After(5 * time.Second):
		t.Fatal("stopServers did not return within 5s")
	}
}

// ---- U: full Start/Stop with an http listener too --------------------------

func TestStartStopWithHttpListener(t *testing.T) {
	withTempHealthPorts(t)
	sched := newMockScheduler()
	m := newStartupTestMR(t)
	m.scheduler = sched
	m.cronDisabled = false
	// subapp version metric is package-level (F1 fix).

	ports := pickFreePorts(t, 2)
	httpPort := ports[0]
	grpcPort := ports[1]

	app := &startupTestApp{
		name: "withhttp",
		listeners: []ListenerSpec{
			{Name: "grpc", Protocol: "grpc", PreferredPort: grpcPort},
			{Name: "http", Protocol: "http", PreferredPort: httpPort},
		},
	}
	if err := m.processSubApp(app, OnPorts(PortMap{
		"grpc": grpcPort,
		"http": httpPort,
	})); err != nil {
		t.Fatalf("processSubApp: %v", err)
	}
	m.subApps = append(m.subApps, registeredSubApp{
		app:     app,
		portMap: PortMap{"grpc": grpcPort, "http": httpPort},
	})

	ctx, cancel := context.WithCancel(context.Background())
	startDone := make(chan error, 1)
	go func() { startDone <- m.Start(ctx) }()

	// Wait for readiness.
	deadline := time.Now().Add(3 * time.Second)
	readyURL := fmt.Sprintf("http://127.0.0.1:%d/health/ready", healthListenPort)
	for {
		if time.Now().After(deadline) {
			t.Fatal("/health/ready never returned 200 in time")
		}
		resp, err := http.Get(readyURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Both ports should accept connections.
	if err := dialTCP(grpcPort, 500*time.Millisecond); err != nil {
		t.Errorf("dial grpc: %v", err)
	}
	if err := dialTCP(httpPort, 500*time.Millisecond); err != nil {
		t.Errorf("dial http: %v", err)
	}

	cancel()
	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("Start returned: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5s")
	}
}
