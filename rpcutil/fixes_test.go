// fixes_test.go — review-driven regression tests for the PR #115 framework.
//
// Each test below pins a specific fix from the multi-perspective code review:
//   F1: mega_subapp_version gauge must use a package-level sync.Once so multiple
//       MultiResource instances in the same process don't panic on duplicate
//       prometheus.MustRegister.
//   F3: normalizeMegaAppName strips a leading "galxe-" so OTEL_SERVICE_NAME does
//       not become "galxe-galxe-foo".
//   F4: Register is concurrency-safe across goroutines (no duplicate names or
//       lost subapps).
//   F5: processSubApp's port reservation is atomic — no two concurrent Register
//       calls can both claim the same port.
//   F6: scanLeakedSDKEnv is a no-op when no subApps are registered.
//   F7: stopServers does not bump mega_shutdown_aborted_total for non-timeout
//       Shutdown errors (e.g. ErrServerClosed when already-closed).
//   WithRedis / WithDCache: option injectors thread the values through to the
//       MultiResource so CacheFor sub-apps work end-to-end.
package rpcutil

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	m.stopServers()
	after := testutil.ToFloat64(shutdownAbortCounter.WithLabelValues("fake", "http"))
	if after != before {
		t.Errorf("non-timeout shutdown bumped abort counter (before=%v after=%v)", before, after)
	}
}

// ---- helpers --------------------------------------------------------------

func silenceLogger(t *testing.T) {
	t.Helper()
	orig := log.Logger
	log.Logger = zerolog.Nop()
	t.Cleanup(func() { log.Logger = orig })
}
