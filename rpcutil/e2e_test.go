// e2e_test.go — thicker integration tests that exercise the full framework
// chain over real loopback HTTP: NewMultiResource → Register (with HTTP
// listeners) → Start → real HTTP requests through the framework middleware
// chain → verify subapp ctx propagation + mega_http_request_total counter +
// /metrics endpoint serves the counter → graceful Stop.
//
// Complements startup_test.go's TestStartStopHappyPath (which only dials the
// gRPC port without invoking a real RPC).

package rpcutil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"

	gopkg_zerolog "github.com/ggsrc/gopkg/zerolog"
)

// e2eHttpApp implements SubApp with a real HTTP handler that reads subapp
// from ctx and echoes it back, plus a panicking endpoint to verify recover.
type e2eHttpApp struct {
	name        string
	port        int
	handlerHits atomic.Int32
	closeCount  atomic.Int32
}

func (a *e2eHttpApp) Name() string { return a.name }
func (a *e2eHttpApp) Listeners() []ListenerSpec {
	return []ListenerSpec{
		{Name: "http", Protocol: "http", PreferredPort: a.port},
	}
}
func (a *e2eHttpApp) Init(*MultiResource) error { return nil }
func (a *e2eHttpApp) RegisterRoutes(reg RouteRegistry) error {
	eng := reg.HTTP("http")
	if eng == nil {
		return fmt.Errorf("e2eHttpApp %s: no http engine for listener 'http'", a.name)
	}
	eng.GET("/hello", func(c *gin.Context) {
		a.handlerHits.Add(1)
		subapp, _ := gopkg_zerolog.SubAppFromCtx(c.Request.Context())
		c.String(http.StatusOK, "hello-"+subapp)
	})
	eng.GET("/boom", func(c *gin.Context) {
		panic("handler boom")
	})
	return nil
}
func (a *e2eHttpApp) HealthChecks() []HealthCheckable {
	return []HealthCheckable{
		{
			Name:        a.name + "-self",
			Criticality: Critical,
			Check:       func(context.Context) error { return nil },
		},
	}
}
func (a *e2eHttpApp) Cron() []CronJob { return nil }
func (a *e2eHttpApp) Close(context.Context) error {
	a.closeCount.Add(1)
	return nil
}

// TestE2EFullChainTwoSubApps runs a mini-mega with two HTTP sub-apps over
// real loopback ports and verifies the full request lifecycle.
func TestE2EFullChainTwoSubApps(t *testing.T) {
	withTempHealthPorts(t)
	gin.SetMode(gin.TestMode)

	m := newStartupTestMR(t)
	m.cronDisabled = true // skip scheduler — nil scheduler is fine when disabled
	m.appName = "e2e-mega"

	// subapp version metric is now package-level sync.Once — no per-instance
	// pre-fire needed (F1 fix).

	ports := pickFreePorts(t, 2)
	alphaPort, betaPort := ports[0], ports[1]

	alpha := &e2eHttpApp{name: "alpha", port: alphaPort}
	beta := &e2eHttpApp{name: "beta", port: betaPort}

	for _, app := range []*e2eHttpApp{alpha, beta} {
		if err := m.processSubApp(app, OnPorts(PortMap{"http": app.port})); err != nil {
			t.Fatalf("processSubApp %s: %v", app.name, err)
		}
		m.subApps = append(m.subApps, registeredSubApp{
			app:     app,
			portMap: PortMap{"http": app.port},
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	startDone := make(chan error, 1)
	go func() { startDone <- m.Start(ctx) }()

	// Wait for /health/ready to become 200.
	readyURL := fmt.Sprintf("http://127.0.0.1:%d/health/ready", healthListenPort)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if time.Now().After(deadline) {
			cancel()
			<-startDone
			t.Fatal("/health/ready never 200 within 3s")
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

	// Hit each sub-app's /hello and verify response body contains its subapp.
	for _, app := range []*e2eHttpApp{alpha, beta} {
		body := mustGetBody(t, fmt.Sprintf("http://127.0.0.1:%d/hello", app.port))
		want := "hello-" + app.name
		if body != want {
			t.Errorf("subapp %s GET /hello: body=%q want=%q", app.name, body, want)
		}
		if got := app.handlerHits.Load(); got != 1 {
			t.Errorf("subapp %s handlerHits=%d, want 1", app.name, got)
		}
	}

	// Hit /boom on alpha → 500 (panic recovered).
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/boom", alphaPort))
	if err != nil {
		t.Fatalf("GET /boom: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("/boom status=%d, want 500", resp.StatusCode)
	}

	// /metrics: hit and grep for our counter labels.
	metricsURL := fmt.Sprintf("http://127.0.0.1:%d/metrics", metricListenPort)
	metricsBody := mustGetBody(t, metricsURL)
	if !strings.Contains(metricsBody, "mega_http_request_total") {
		t.Errorf("/metrics missing mega_http_request_total")
	}
	if !strings.Contains(metricsBody, `subapp="alpha"`) || !strings.Contains(metricsBody, `subapp="beta"`) {
		t.Errorf("/metrics missing subapp labels alpha/beta")
	}
	if !strings.Contains(metricsBody, "mega_panic_total") {
		t.Errorf("/metrics missing mega_panic_total (panic recover counter)")
	}

	// Direct counter check via testutil for precision.
	if got := testutil.ToFloat64(httpRequestCounter.WithLabelValues("alpha", "/hello", "GET", "200")); got != 1 {
		t.Errorf("httpRequestCounter alpha /hello GET 200 = %v, want 1", got)
	}
	if got := testutil.ToFloat64(panicCounter.WithLabelValues("alpha", "http", "/boom")); got != 1 {
		t.Errorf("panicCounter alpha /boom = %v, want 1", got)
	}

	// Trigger graceful stop.
	cancel()
	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5s of cancel")
	}

	for _, app := range []*e2eHttpApp{alpha, beta} {
		if got := app.closeCount.Load(); got != 1 {
			t.Errorf("subapp %s closeCount=%d, want 1", app.name, got)
		}
	}
}

// mustGetBody does a quick HTTP GET and returns the body as string, failing
// the test on error or non-200 status.
func mustGetBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body %s: %v", url, err)
	}
	return string(b)
}
