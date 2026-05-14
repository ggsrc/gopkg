// listener_test.go — Wave 1 agent B tests for port allocation + multi-listener
// server factory. White-box (package rpcutil) to access private types/methods.
package rpcutil

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

// ---- helpers -----------------------------------------------------------

// newListenerTestMR builds a minimal MultiResource with the maps owned by
// listener.go eagerly initialised so per-file methods can be exercised without
// depending on NewMultiResource (still a Wave 3 stub).
//
// Named distinctly from cron_test.go / health_test.go helpers to avoid
// cross-file collisions during the Wave 1 parallel build.
func newListenerTestMR() *MultiResource {
	return &MultiResource{
		portMap:     map[string]int{},
		grpcServers: map[int]*grpc.Server{},
		httpRouters: map[int]*gin.Engine{},
	}
}

// fakeSubApp is a minimal SubApp impl used to drive processSubApp tests.
type fakeSubApp struct {
	name      string
	listeners []ListenerSpec
}

func (f *fakeSubApp) Name() string                           { return f.name }
func (f *fakeSubApp) Listeners() []ListenerSpec              { return f.listeners }
func (f *fakeSubApp) Init(deps *MultiResource) error         { return nil }
func (f *fakeSubApp) RegisterRoutes(reg RouteRegistry) error { return nil }
func (f *fakeSubApp) HealthChecks() []HealthCheckable        { return nil }
func (f *fakeSubApp) Cron() []CronJob                        { return nil }
func (f *fakeSubApp) Close(ctx context.Context) error        { return nil }

// ---- OnPorts / WithRequirePreferredPort / mergeRegisterOptions ---------

func TestOnPortsSetsPortMap(t *testing.T) {
	pm := PortMap{"grpc": 9090}
	cfg := mergeRegisterOptions(OnPorts(pm))
	if cfg.portMap == nil {
		t.Fatal("expected portMap set")
	}
	if cfg.portMap["grpc"] != 9090 {
		t.Errorf("expected grpc=9090, got %d", cfg.portMap["grpc"])
	}
	if cfg.requirePreferred {
		t.Error("requirePreferred should default false")
	}
}

func TestWithRequirePreferredPort(t *testing.T) {
	cfg := mergeRegisterOptions(WithRequirePreferredPort())
	if !cfg.requirePreferred {
		t.Error("expected requirePreferred=true")
	}
}

func TestMergeRegisterOptionsCombines(t *testing.T) {
	cfg := mergeRegisterOptions(
		OnPorts(PortMap{"grpc": 9090}),
		WithRequirePreferredPort(),
	)
	if cfg.portMap["grpc"] != 9090 || !cfg.requirePreferred {
		t.Errorf("merge failed: %+v", cfg)
	}
}

func TestMergeRegisterOptionsEmpty(t *testing.T) {
	cfg := mergeRegisterOptions()
	if cfg.portMap != nil {
		t.Errorf("expected nil portMap, got %+v", cfg.portMap)
	}
	if cfg.requirePreferred {
		t.Error("expected requirePreferred=false")
	}
}

// ---- processSubApp -----------------------------------------------------

func TestProcessSubAppAllListenersAssigned(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newListenerTestMR()
	app := &fakeSubApp{
		name: "airdrop",
		listeners: []ListenerSpec{
			{Name: "grpc", Protocol: "grpc", PreferredPort: 9090},
			{Name: "http-rest", Protocol: "http", PreferredPort: 3000},
		},
	}
	err := m.processSubApp(app, OnPorts(PortMap{"grpc": 9090, "http-rest": 3000}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if m.portMap["airdrop.grpc"] != 9090 {
		t.Errorf("portMap airdrop.grpc want 9090 got %d", m.portMap["airdrop.grpc"])
	}
	if m.portMap["airdrop.http-rest"] != 3000 {
		t.Errorf("portMap airdrop.http-rest want 3000 got %d", m.portMap["airdrop.http-rest"])
	}
	if _, ok := m.grpcServers[9090]; !ok {
		t.Error("grpc server not created on 9090")
	}
	if _, ok := m.httpRouters[3000]; !ok {
		t.Error("http router not created on 3000")
	}
}

func TestProcessSubAppMissingPortInPortMap(t *testing.T) {
	m := newListenerTestMR()
	app := &fakeSubApp{
		name: "airdrop",
		listeners: []ListenerSpec{
			{Name: "grpc", Protocol: "grpc", PreferredPort: 9090},
			{Name: "http-rest", Protocol: "http", PreferredPort: 3000},
		},
	}
	err := m.processSubApp(app, OnPorts(PortMap{"grpc": 9090})) // missing http-rest
	if err == nil {
		t.Fatal("expected error for missing listener port")
	}
	msg := err.Error()
	if !strings.Contains(msg, "airdrop") || !strings.Contains(msg, "http-rest") {
		t.Errorf("error should mention subapp + listener; got %q", msg)
	}
}

func TestProcessSubAppPortAlreadyTaken(t *testing.T) {
	m := newListenerTestMR()
	// Pre-occupy port 9090 with another subapp.
	first := &fakeSubApp{
		name:      "airdrop",
		listeners: []ListenerSpec{{Name: "grpc", Protocol: "grpc", PreferredPort: 9090}},
	}
	if err := m.processSubApp(first, OnPorts(PortMap{"grpc": 9090})); err != nil {
		t.Fatalf("first register: %v", err)
	}
	second := &fakeSubApp{
		name:      "token",
		listeners: []ListenerSpec{{Name: "grpc", Protocol: "grpc", PreferredPort: 9090}},
	}
	err := m.processSubApp(second, OnPorts(PortMap{"grpc": 9090}))
	if err == nil {
		t.Fatal("expected port-taken error")
	}
	if !strings.Contains(err.Error(), "already taken") {
		t.Errorf("error should mention already taken; got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "airdrop") {
		t.Errorf("error should mention conflicting subapp airdrop; got %q", err.Error())
	}
}

func TestProcessSubAppRequirePreferredMismatch(t *testing.T) {
	m := newListenerTestMR()
	app := &fakeSubApp{
		name:      "airdrop",
		listeners: []ListenerSpec{{Name: "grpc", Protocol: "grpc", PreferredPort: 9090}},
	}
	err := m.processSubApp(app,
		OnPorts(PortMap{"grpc": 9999}),
		WithRequirePreferredPort(),
	)
	if err == nil {
		t.Fatal("expected error for requirePreferred mismatch")
	}
	if !strings.Contains(err.Error(), "9090") || !strings.Contains(err.Error(), "9999") {
		t.Errorf("error should mention both ports; got %q", err.Error())
	}
}

func TestProcessSubAppRequirePreferredMatchOK(t *testing.T) {
	m := newListenerTestMR()
	app := &fakeSubApp{
		name:      "airdrop",
		listeners: []ListenerSpec{{Name: "grpc", Protocol: "grpc", PreferredPort: 9090}},
	}
	err := m.processSubApp(app,
		OnPorts(PortMap{"grpc": 9090}),
		WithRequirePreferredPort(),
	)
	if err != nil {
		t.Fatalf("expected success when port == preferred; got %v", err)
	}
}

func TestProcessSubAppPortMapNilFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newListenerTestMR()
	app := &fakeSubApp{
		name: "airdrop",
		listeners: []ListenerSpec{
			{Name: "grpc", Protocol: "grpc", PreferredPort: 9090},
			{Name: "http-rest", Protocol: "http", PreferredPort: 3000},
		},
	}
	if err := m.processSubApp(app); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if m.portMap["airdrop.grpc"] != 9090 {
		t.Errorf("fallback should use PreferredPort 9090, got %d", m.portMap["airdrop.grpc"])
	}
	if m.portMap["airdrop.http-rest"] != 3000 {
		t.Errorf("fallback should use PreferredPort 3000, got %d", m.portMap["airdrop.http-rest"])
	}
}

func TestProcessSubAppOverrideAcceptedNoRequire(t *testing.T) {
	m := newListenerTestMR()
	app := &fakeSubApp{
		name:      "airdrop",
		listeners: []ListenerSpec{{Name: "grpc", Protocol: "grpc", PreferredPort: 9090}},
	}
	// port differs from preferred but requirePreferred is false → accepted (warn only).
	if err := m.processSubApp(app, OnPorts(PortMap{"grpc": 9999})); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if m.portMap["airdrop.grpc"] != 9999 {
		t.Errorf("override should be accepted; got %d", m.portMap["airdrop.grpc"])
	}
}

func TestProcessSubAppUnknownProtocol(t *testing.T) {
	m := newListenerTestMR()
	app := &fakeSubApp{
		name:      "weird",
		listeners: []ListenerSpec{{Name: "ws", Protocol: "websocket", PreferredPort: 7000}},
	}
	err := m.processSubApp(app, OnPorts(PortMap{"ws": 7000}))
	if err == nil {
		t.Fatal("expected error for unknown protocol")
	}
	if !strings.Contains(err.Error(), "websocket") {
		t.Errorf("error should mention protocol; got %q", err.Error())
	}
}

// ---- GrpcServerOn / HttpRouterOn --------------------------------------

func TestGrpcServerOnIdempotent(t *testing.T) {
	m := newListenerTestMR()
	s1 := m.GrpcServerOn(9090)
	s2 := m.GrpcServerOn(9090)
	if s1 == nil || s2 == nil {
		t.Fatal("nil server returned")
	}
	if s1 != s2 {
		t.Error("expected same *grpc.Server pointer on repeat call")
	}
}

func TestGrpcServerOnLazyInitsMap(t *testing.T) {
	// portMap is the only map we eagerly set; grpcServers map should be
	// lazily initialised even if nil.
	m := &MultiResource{}
	s := m.GrpcServerOn(9090)
	if s == nil {
		t.Fatal("nil server")
	}
	if m.grpcServers[9090] != s {
		t.Error("grpcServers not populated")
	}
}

func TestHttpRouterOnIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newListenerTestMR()
	e1 := m.HttpRouterOn(3000)
	e2 := m.HttpRouterOn(3000)
	if e1 == nil || e2 == nil {
		t.Fatal("nil engine returned")
	}
	if e1 != e2 {
		t.Error("expected same *gin.Engine pointer on repeat call")
	}
}

func TestHttpRouterOnLazyInitsMap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := &MultiResource{}
	e := m.HttpRouterOn(3000)
	if e == nil {
		t.Fatal("nil engine")
	}
	if m.httpRouters[3000] != e {
		t.Error("httpRouters not populated")
	}
}

// ---- subappForListener -------------------------------------------------

func TestSubappForListenerKnown(t *testing.T) {
	m := newListenerTestMR()
	app := &fakeSubApp{
		name:      "airdrop",
		listeners: []ListenerSpec{{Name: "grpc", Protocol: "grpc", PreferredPort: 9090}},
	}
	if err := m.processSubApp(app, OnPorts(PortMap{"grpc": 9090})); err != nil {
		t.Fatalf("register: %v", err)
	}
	if got := m.subappForListener(9090); got != "airdrop" {
		t.Errorf("subappForListener(9090) = %q, want airdrop", got)
	}
}

func TestSubappForListenerUnknown(t *testing.T) {
	m := newListenerTestMR()
	if got := m.subappForListener(12345); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// ---- concurrency ------------------------------------------------------

func TestProcessSubAppParallelDisjointPorts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newListenerTestMR()
	const n = 10
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			app := &fakeSubApp{
				name: fakeName(i),
				listeners: []ListenerSpec{
					{Name: "grpc", Protocol: "grpc", PreferredPort: 9000 + i},
				},
			}
			if err := m.processSubApp(app, OnPorts(PortMap{"grpc": 9000 + i})); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("parallel processSubApp error: %v", err)
	}
	if len(m.portMap) != n {
		t.Errorf("expected %d portMap entries, got %d", n, len(m.portMap))
	}
	if len(m.grpcServers) != n {
		t.Errorf("expected %d grpcServers, got %d", n, len(m.grpcServers))
	}
}

func fakeName(i int) string {
	return "sub-" + string(rune('a'+i))
}
