// health_test.go — white-box tests for Wave 1 agent C: health aggregation.
package rpcutil

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// newTestMR builds a usable MultiResource for tests (bypassing the
// errNotImplemented NewMultiResource stub).
func newTestMR() *MultiResource {
	return &MultiResource{
		appName:   "test-mega",
		startedAt: &atomic.Pointer[time.Time]{},
	}
}

func markStarted(m *MultiResource) {
	now := time.Now()
	m.startedAt.Store(&now)
}

func okCheck() func(ctx context.Context) error {
	return func(ctx context.Context) error { return nil }
}

func errCheck(msg string) func(ctx context.Context) error {
	return func(ctx context.Context) error { return errors.New(msg) }
}

// slowCheck blocks until ctx is done (or sleep elapses), then returns ctx.Err.
func slowCheck(d time.Duration) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		select {
		case <-time.After(d):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func readBody(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	b, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

// ---- A. registerHealthCheck appends & is race-safe ------------------------

func TestRegisterHealthCheck_Appends(t *testing.T) {
	m := newTestMR()
	m.registerHealthCheck("airdrop", []HealthCheckable{
		{Name: "db", Criticality: Critical, Check: okCheck()},
		{Name: "redis", Criticality: Optional, Check: okCheck()},
	})
	if len(m.healthChecks) != 2 {
		t.Fatalf("want 2 health checks, got %d", len(m.healthChecks))
	}
	if m.healthChecks[0].subapp != "airdrop" || m.healthChecks[0].hc.Name != "db" {
		t.Errorf("unexpected first entry: %+v", m.healthChecks[0])
	}
}

func TestRegisterHealthCheck_Concurrent(t *testing.T) {
	m := newTestMR()
	var wg sync.WaitGroup
	const goroutines = 16
	const perG = 8
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			checks := make([]HealthCheckable, perG)
			for j := 0; j < perG; j++ {
				checks[j] = HealthCheckable{
					Name:        "dep",
					Criticality: Critical,
					Check:       okCheck(),
				}
			}
			m.registerHealthCheck("sub", checks)
		}()
	}
	wg.Wait()
	if got, want := len(m.healthChecks), goroutines*perG; got != want {
		t.Fatalf("want %d entries after concurrent registers, got %d", want, got)
	}
}

// ---- B/C/D/E/F. readinessHandler ------------------------------------------

func TestReadiness_Starting(t *testing.T) {
	m := newTestMR() // startedAt set but no value Stored.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	m.readinessHandler(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
	if !strings.Contains(readBody(t, rr), "starting") {
		t.Errorf("want body to contain 'starting'")
	}
}

func TestReadiness_StartedAtNil(t *testing.T) {
	m := &MultiResource{appName: "test-mega"} // startedAt pointer itself nil.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	m.readinessHandler(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
	if !strings.Contains(readBody(t, rr), "starting") {
		t.Errorf("want 'starting' body")
	}
}

func TestReadiness_AllCriticalPass(t *testing.T) {
	m := newTestMR()
	markStarted(m)
	m.registerHealthCheck("airdrop", []HealthCheckable{
		{Name: "db", Criticality: Critical, Check: okCheck()},
		{Name: "redis", Criticality: Critical, Check: okCheck()},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	m.readinessHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%q", rr.Code, readBody(t, rr))
	}
	if !strings.Contains(readBody(t, rr), "ready") {
		t.Errorf("want 'ready' body")
	}
}

func TestReadiness_OneCriticalFails(t *testing.T) {
	m := newTestMR()
	markStarted(m)
	m.registerHealthCheck("airdrop", []HealthCheckable{
		{Name: "db", Criticality: Critical, Check: errCheck("conn refused")},
		{Name: "redis", Criticality: Critical, Check: okCheck()},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	m.readinessHandler(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
	body := readBody(t, rr)
	if !strings.Contains(body, "unhealthy critical deps") {
		t.Errorf("want 'unhealthy critical deps' in body, got %q", body)
	}
	if !strings.Contains(body, "airdrop.db") {
		t.Errorf("want subapp.dep_name in body, got %q", body)
	}
	if !strings.Contains(body, "conn refused") {
		t.Errorf("want error message in body, got %q", body)
	}
}

func TestReadiness_OptionalFailDoesNotAffectStatus(t *testing.T) {
	m := newTestMR()
	markStarted(m)
	m.registerHealthCheck("airdrop", []HealthCheckable{
		{Name: "critical-dep", Criticality: Critical, Check: okCheck()},
		{Name: "optional-dep", Criticality: Optional, Check: errCheck("flaky")},
	})

	before := testutil.ToFloat64(
		depUnhealthyCounter.WithLabelValues("airdrop", "optional-dep", "optional"),
	)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	m.readinessHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 (optional fail must not flip status), got %d body=%q",
			rr.Code, readBody(t, rr))
	}

	after := testutil.ToFloat64(
		depUnhealthyCounter.WithLabelValues("airdrop", "optional-dep", "optional"),
	)
	if after-before != 1 {
		t.Errorf("optional-dep counter delta = %v, want 1", after-before)
	}
}

func TestReadiness_TimeoutCountsAsFailure(t *testing.T) {
	m := newTestMR()
	markStarted(m)
	m.registerHealthCheck("staking", []HealthCheckable{
		// Sleeps longer than the 2s readiness timeout → ctx deadline error.
		{Name: "slow-dep", Criticality: Critical, Check: slowCheck(3 * time.Second)},
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	start := time.Now()
	m.readinessHandler(rr, req)
	elapsed := time.Since(start)
	if elapsed > 3*time.Second {
		t.Fatalf("readinessHandler did not honor 2s timeout, elapsed=%v", elapsed)
	}
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 after timeout, got %d", rr.Code)
	}
	if !strings.Contains(readBody(t, rr), "staking.slow-dep") {
		t.Errorf("want failure for staking.slow-dep, got %q", readBody(t, rr))
	}
}

// ---- G. liveness ----------------------------------------------------------

func TestLiveness_AlwaysOK(t *testing.T) {
	m := &MultiResource{} // no startedAt; should still respond 200.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	m.livenessHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("liveness want 200, got %d", rr.Code)
	}
	if !strings.Contains(readBody(t, rr), "alive") {
		t.Errorf("liveness want 'alive', got %q", readBody(t, rr))
	}
}

// ---- H. debugHandler ------------------------------------------------------

func TestDebugHandler_JSONShape(t *testing.T) {
	m := newTestMR()
	markStarted(m)
	// Two sub-apps, registered out-of-order to verify alpha sort.
	m.registerHealthCheck("staking", []HealthCheckable{
		{Name: "staking-db", Criticality: Critical, Check: okCheck()},
	})
	m.registerHealthCheck("airdrop", []HealthCheckable{
		{Name: "airdrop-db", Criticality: Critical, Check: okCheck()},
		{Name: "airdrop-redis", Criticality: Optional, Check: errCheck("redis down")},
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/debug", nil)
	m.debugHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("debug want 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp struct {
		MegaName  string     `json:"mega_name"`
		StartedAt *time.Time `json:"started_at"`
		SubApps   []struct {
			Name         string `json:"name"`
			HealthChecks []struct {
				Name        string `json:"name"`
				Criticality string `json:"criticality"`
				Status      string `json:"status"`
				Error       string `json:"error,omitempty"`
				DurationMs  int64  `json:"duration_ms"`
			} `json:"health_checks"`
		} `json:"subapps"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode debug json: %v body=%s", err, rr.Body.String())
	}
	if resp.MegaName != "test-mega" {
		t.Errorf("mega_name = %q, want test-mega", resp.MegaName)
	}
	if resp.StartedAt == nil {
		t.Errorf("started_at should be non-nil")
	}
	if len(resp.SubApps) != 2 {
		t.Fatalf("want 2 subapps, got %d", len(resp.SubApps))
	}
	// Alphabetical: airdrop < staking.
	if resp.SubApps[0].Name != "airdrop" || resp.SubApps[1].Name != "staking" {
		t.Errorf("subapps not sorted: %v / %v", resp.SubApps[0].Name, resp.SubApps[1].Name)
	}
	// airdrop has 2 checks: one ok, one error.
	airdrop := resp.SubApps[0]
	if len(airdrop.HealthChecks) != 2 {
		t.Fatalf("airdrop want 2 checks, got %d", len(airdrop.HealthChecks))
	}
	var sawOK, sawErr bool
	for _, c := range airdrop.HealthChecks {
		switch c.Name {
		case "airdrop-db":
			if c.Status != "ok" {
				t.Errorf("airdrop-db status = %q, want ok", c.Status)
			}
			if c.Error != "" {
				t.Errorf("airdrop-db error should be empty, got %q", c.Error)
			}
			if c.Criticality != "critical" {
				t.Errorf("airdrop-db criticality = %q", c.Criticality)
			}
			sawOK = true
		case "airdrop-redis":
			if c.Status != "error" {
				t.Errorf("airdrop-redis status = %q, want error", c.Status)
			}
			if c.Error != "redis down" {
				t.Errorf("airdrop-redis error = %q", c.Error)
			}
			if c.Criticality != "optional" {
				t.Errorf("airdrop-redis criticality = %q", c.Criticality)
			}
			sawErr = true
		}
	}
	if !sawOK || !sawErr {
		t.Errorf("missing ok or err check in airdrop subapp: %+v", airdrop)
	}
}

func TestDebugHandler_NoStartedAt(t *testing.T) {
	m := &MultiResource{appName: "x"} // pointer nil
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/debug", nil)
	m.debugHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got["mega_name"] != "x" {
		t.Errorf("mega_name = %v", got["mega_name"])
	}
}

// ---- I. healthMux routing -------------------------------------------------

func TestHealthMux_Routes(t *testing.T) {
	m := newTestMR()
	markStarted(m)
	m.registerHealthCheck("airdrop", []HealthCheckable{
		{Name: "db", Criticality: Critical, Check: okCheck()},
	})
	mux := m.healthMux()

	cases := []struct {
		path        string
		wantStatus  int
		wantBody    string
		wantContent string
	}{
		{"/health/live", 200, "alive", ""},
		{"/health/ready", 200, "ready", ""},
		{"/health/debug", 200, "", "application/json"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			mux.ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("path=%s code=%d want=%d body=%q",
					tc.path, rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.wantBody != "" && !strings.Contains(rr.Body.String(), tc.wantBody) {
				t.Errorf("path=%s body=%q want contains %q",
					tc.path, rr.Body.String(), tc.wantBody)
			}
			if tc.wantContent != "" && rr.Header().Get("Content-Type") != tc.wantContent {
				t.Errorf("path=%s Content-Type=%q want=%q",
					tc.path, rr.Header().Get("Content-Type"), tc.wantContent)
			}
		})
	}
}
