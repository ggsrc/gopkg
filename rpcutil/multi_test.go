// multi_test.go — Wave 3 agent G tests for MultiResource factory, Register
// dispatch, ctx / logger / metric helpers, and SDK env shielding.
//
// White-box (package rpcutil) to poke private fields / helpers.
package rpcutil

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	gopkg_zerolog "github.com/ggsrc/gopkg/zerolog"
)

// ---- helpers ----------------------------------------------------------

// multiFakeApp implements SubApp with optional health + cron payload.
// Distinct from fakeSubApp in listener_test.go to avoid coupling test plans.
type multiFakeApp struct {
	name        string
	listeners   []ListenerSpec
	healthSpecs []HealthCheckable
	cronSpecs   []CronJob
}

func (f *multiFakeApp) Name() string                           { return f.name }
func (f *multiFakeApp) Listeners() []ListenerSpec              { return f.listeners }
func (f *multiFakeApp) Init(deps *MultiResource) error         { return nil }
func (f *multiFakeApp) RegisterRoutes(reg RouteRegistry) error { return nil }
func (f *multiFakeApp) HealthChecks() []HealthCheckable        { return f.healthSpecs }
func (f *multiFakeApp) Cron() []CronJob                        { return f.cronSpecs }
func (f *multiFakeApp) Close(ctx context.Context) error        { return nil }

func newMultiFakeApp(name string, ports ...int) *multiFakeApp {
	specs := make([]ListenerSpec, 0, len(ports))
	for i, p := range ports {
		specs = append(specs, ListenerSpec{
			Name:          "l" + itoa(i),
			Protocol:      "grpc",
			PreferredPort: p,
		})
	}
	return &multiFakeApp{name: name, listeners: specs}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	out := ""
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	for i > 0 {
		out = string(rune('0'+i%10)) + out
		i /= 10
	}
	if neg {
		out = "-" + out
	}
	return out
}

// ---- A: NewMultiResource validation ----------------------------------

func TestNewMultiResourceRequiresAppName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, err := NewMultiResource(context.Background())
	if err == nil {
		t.Fatal("expected error when appName missing")
	}
	if !strings.Contains(err.Error(), "WithMegaAppName") {
		t.Errorf("error must mention WithMegaAppName, got: %v", err)
	}
}

// ---- B: happy path initialisation ------------------------------------

func TestNewMultiResourceHappyPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Avoid bleed from previous test runs.
	t.Setenv("MEGA_DISABLE_CRON", "")
	m, err := NewMultiResource(context.Background(), WithMegaAppName("airdrop-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}
	if m.appName != "airdrop-mega" {
		t.Errorf("appName = %q", m.appName)
	}
	if m.rootLogger == nil {
		t.Error("rootLogger nil")
	}
	if !m.migrationMode {
		t.Error("migrationMode default should be true")
	}
	if m.portMap == nil || m.grpcServers == nil || m.grpcListeners == nil ||
		m.httpRouters == nil || m.httpListeners == nil || m.httpServers == nil ||
		m.dbPools == nil || m.dbConfigs == nil ||
		m.registries == nil || m.cronJobs == nil {
		t.Error("expected all maps non-nil")
	}
	if m.scheduler == nil {
		t.Error("scheduler nil")
	}
	if m.startedAt == nil {
		t.Error("startedAt nil")
	}
	if m.startedAt.Load() != nil {
		t.Error("startedAt should not yet be set")
	}
	if m.cronDisabled {
		t.Error("cronDisabled should default false without MEGA_DISABLE_CRON")
	}
	if len(m.gatherers) == 0 {
		t.Error("gatherers should include DefaultGatherer")
	}
}

// ---- C: MEGA_DISABLE_CRON env ----------------------------------------

func TestNewMultiResourceCronDisabledFromEnv(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("MEGA_DISABLE_CRON", "true")
	m, err := NewMultiResource(context.Background(), WithMegaAppName("mega-test"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}
	if !m.cronDisabled {
		t.Error("cronDisabled should be true when MEGA_DISABLE_CRON=true")
	}
}

// ---- D: WithMigrationMode --------------------------------------------

func TestWithMigrationModeDisable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m, err := NewMultiResource(
		context.Background(),
		WithMegaAppName("m"),
		WithMigrationMode(false),
	)
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}
	if m.migrationMode {
		t.Error("WithMigrationMode(false) should disable migrationMode")
	}
}

// ---- E: setMegaSDKEnv sets OTEL_SERVICE_NAME -------------------------

func TestSetMegaSDKEnvForcesServiceName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("OTEL_SERVICE_NAME", "stale-old-value")
	m, err := NewMultiResource(context.Background(), WithMegaAppName("airdrop-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}
	_ = m
	if got := os.Getenv("OTEL_SERVICE_NAME"); got != "galxe-airdrop-mega" {
		t.Errorf("OTEL_SERVICE_NAME = %q, want %q", got, "galxe-airdrop-mega")
	}
}

// ---- F: setMegaSDKEnv warns on prefixed leak --------------------------

func TestSetMegaSDKEnvWarnsOnPrefixedLeak(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Capture global zerolog/log output via in-memory writer.
	var buf bytes.Buffer
	orig := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = orig })

	t.Setenv("STAKING_OTEL_SERVICE_NAME", "leaked-value")

	m, err := NewMultiResource(context.Background(), WithMegaAppName("airdrop-mega"))
	if err != nil {
		t.Fatalf("NewMultiResource: %v", err)
	}
	// Register a "staking" sub-app so setMegaSDKEnv's loop sees the prefix.
	if err := m.Register(newMultiFakeApp("staking", 19090), OnPorts(PortMap{"l0": 19090})); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Re-audit after Register so the warn loop has subapp names (F6 fix —
	// leak scan now lives in scanLeakedSDKEnv and runs at Start).
	m.scanLeakedSDKEnv()

	output := buf.String()
	if !strings.Contains(output, "STAKING_OTEL_SERVICE_NAME") {
		t.Errorf("expected warn referencing STAKING_OTEL_SERVICE_NAME, got log: %s", output)
	}
	if !strings.Contains(output, "leaked SDK env") {
		t.Errorf("expected 'leaked SDK env' phrase in log, got: %s", output)
	}
}

// ---- G: Register zero args -------------------------------------------

func TestRegisterZeroArgs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := mustNewMultiTest(t, "mega")
	err := m.Register()
	if err == nil || !strings.Contains(err.Error(), "at least one SubApp") {
		t.Errorf("expected at-least-one-SubApp error, got %v", err)
	}
}

// ---- H: Register single SubApp without options uses PreferredPort -----

func TestRegisterSingleSubAppPreferredPortFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := mustNewMultiTest(t, "mega")
	app := newMultiFakeApp("airdrop", 19191)
	if err := m.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := len(m.subApps); got != 1 {
		t.Fatalf("subApps len = %d, want 1", got)
	}
	if m.subApps[0].envPrefix != "AIRDROP" {
		t.Errorf("envPrefix = %q, want AIRDROP", m.subApps[0].envPrefix)
	}
	if m.portMap["airdrop.l0"] != 19191 {
		t.Errorf("portMap not populated via PreferredPort: %+v", m.portMap)
	}
}

// ---- I: Register SubApp + OnPorts ------------------------------------

func TestRegisterWithOnPorts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := mustNewMultiTest(t, "mega")
	app := newMultiFakeApp("token", 9090)
	if err := m.Register(app, OnPorts(PortMap{"l0": 19292})); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if m.portMap["token.l0"] != 19292 {
		t.Errorf("portMap = %+v, want token.l0=19292", m.portMap)
	}
}

// ---- J: Register two SubApps interleaved + envPrefix dash → underscore

func TestRegisterTwoSubAppsInterleavedDashToUnderscore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := mustNewMultiTest(t, "mega")
	a := newMultiFakeApp("chain-adapter", 19390)
	b := newMultiFakeApp("token", 19391)

	err := m.Register(
		a, OnPorts(PortMap{"l0": 19390}),
		b, OnPorts(PortMap{"l0": 19391}),
	)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(m.subApps) != 2 {
		t.Fatalf("subApps len = %d", len(m.subApps))
	}
	if m.subApps[0].envPrefix != "CHAIN_ADAPTER" {
		t.Errorf("envPrefix[0] = %q", m.subApps[0].envPrefix)
	}
	if m.subApps[1].envPrefix != "TOKEN" {
		t.Errorf("envPrefix[1] = %q", m.subApps[1].envPrefix)
	}
}

// ---- K: Register arg of wrong type -----------------------------------

func TestRegisterWrongArgType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := mustNewMultiTest(t, "mega")
	err := m.Register("not a sub-app")
	if err == nil {
		t.Fatal("expected error for non-SubApp first arg")
	}
	if !strings.Contains(err.Error(), "arg 0") || !strings.Contains(err.Error(), "SubApp") {
		t.Errorf("error must mention position+SubApp, got %v", err)
	}
}

func TestRegisterWrongArgTypeMidStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := mustNewMultiTest(t, "mega")
	app := newMultiFakeApp("airdrop", 19490)
	err := m.Register(app, "stray-string")
	if err == nil {
		t.Fatal("expected error for non-Option mid-stream")
	}
	if !strings.Contains(err.Error(), "arg 1") {
		t.Errorf("error must mention position, got %v", err)
	}
	if !strings.Contains(err.Error(), "RegisterOption") {
		t.Errorf("error must mention RegisterOption, got %v", err)
	}
}

// ---- L: duplicate SubApp.Name() across calls -------------------------

func TestRegisterDuplicateName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := mustNewMultiTest(t, "mega")
	first := newMultiFakeApp("airdrop", 19590)
	if err := m.Register(first); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	dup := newMultiFakeApp("airdrop", 19591)
	err := m.Register(dup)
	if err == nil || !strings.Contains(err.Error(), "duplicate SubApp name") {
		t.Errorf("expected duplicate-name error, got %v", err)
	}
}

// ---- M: invalid SubApp.Name() ----------------------------------------

func TestRegisterInvalidName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := mustNewMultiTest(t, "mega")
	bad := &multiFakeApp{name: "Bad Name", listeners: []ListenerSpec{{Name: "l0", Protocol: "grpc", PreferredPort: 19690}}}
	err := m.Register(bad)
	if err == nil || !strings.Contains(err.Error(), "invalid SubApp name") {
		t.Errorf("expected invalid-name error, got %v", err)
	}
}

// ---- N: SubAppContext returns ctx with WithSubApp + zerolog field ----

func TestSubAppContextHasSubAppMarkerAndField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	m := &MultiResource{appName: "mega", rootLogger: &logger, migrationMode: true}

	ctx := m.SubAppContext("airdrop")
	got, ok := gopkg_zerolog.SubAppFromCtx(ctx)
	if !ok || got != "airdrop" {
		t.Errorf("SubAppFromCtx = %q, %v", got, ok)
	}

	zerolog.Ctx(ctx).Info().Msg("hi")
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("decode log: %v (raw=%s)", err, buf.String())
	}
	if entry["subapp"] != "airdrop" {
		t.Errorf("subapp field = %v", entry["subapp"])
	}
	if entry["legacy_app"] != "galxe-airdrop" {
		t.Errorf("legacy_app field = %v", entry["legacy_app"])
	}
}

func TestSubAppContextNoLegacyWhenMigrationDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	m := &MultiResource{appName: "mega", rootLogger: &logger, migrationMode: false}

	ctx := m.SubAppContext("airdrop")
	zerolog.Ctx(ctx).Info().Msg("hi")
	var entry map[string]any
	_ = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry)
	if _, has := entry["legacy_app"]; has {
		t.Errorf("legacy_app must be absent when migrationMode=false, got %v", entry)
	}
	if entry["subapp"] != "airdrop" {
		t.Errorf("subapp = %v", entry["subapp"])
	}
}

// ---- O: LoggerFor ----------------------------------------------------

func TestLoggerForFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	m := &MultiResource{rootLogger: &logger, migrationMode: true}

	l := m.LoggerFor("token")
	l.Info().Msg("x")
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	if entry["subapp"] != "token" {
		t.Errorf("subapp = %v", entry["subapp"])
	}
	if entry["legacy_app"] != "galxe-token" {
		t.Errorf("legacy_app = %v", entry["legacy_app"])
	}
}

func TestLoggerForNoLegacyWhenMigrationDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	m := &MultiResource{rootLogger: &logger, migrationMode: false}

	l := m.LoggerFor("token")
	l.Info().Msg("x")
	var entry map[string]any
	_ = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry)
	if _, has := entry["legacy_app"]; has {
		t.Errorf("legacy_app must be absent, got %v", entry)
	}
}

// ---- P: MetricRegistryFor is idempotent per subapp -------------------

func TestMetricRegistryForIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := mustNewMultiTest(t, "mega")

	a := m.MetricRegistryFor("airdrop")
	b := m.MetricRegistryFor("airdrop")
	if a != b {
		t.Error("MetricRegistryFor must return same registry for same subapp")
	}

	c := m.MetricRegistryFor("token")
	if c == a {
		t.Error("distinct subapps must have distinct registries")
	}
}

// ---- Q: MetricRegistryFor metric gatherable via m.gatherers ----------

func TestMetricRegistryForGatherable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := mustNewMultiTest(t, "mega")

	reg := m.MetricRegistryFor("airdrop")
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rpcutil_multi_test_marker",
		Help: "test gauge for gatherers verification",
	})
	if err := reg.Register(gauge); err != nil {
		t.Fatalf("register gauge: %v", err)
	}
	gauge.Set(42)

	mfs, err := m.gatherers.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	found := false
	for _, mf := range mfs {
		if mf.GetName() == "rpcutil_multi_test_marker" {
			found = true
			break
		}
	}
	if !found {
		t.Error("gauge not surfaced via m.gatherers; subapp registry not wired")
	}
}

// ---- R: subAppNames preserves Register order -------------------------

func TestSubAppNamesPreservesRegisterOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := mustNewMultiTest(t, "mega")

	a := newMultiFakeApp("zebra", 19790)
	b := newMultiFakeApp("alpha", 19791)
	c := newMultiFakeApp("middle", 19792)
	if err := m.Register(
		a, OnPorts(PortMap{"l0": 19790}),
		b, OnPorts(PortMap{"l0": 19791}),
		c, OnPorts(PortMap{"l0": 19792}),
	); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got := m.subAppNames()
	want := []string{"zebra", "alpha", "middle"}
	if len(got) != len(want) {
		t.Fatalf("subAppNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("subAppNames[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// ---- S: registerSubAppVersionMetric idempotent -----------------------

func TestRegisterSubAppVersionMetricIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := mustNewMultiTest(t, "mega-version-test")

	// First call registers a new GaugeVec on DefaultRegisterer; calling
	// twice must not panic (sync.Once guard).
	m.registerSubAppVersionMetric()
	m.registerSubAppVersionMetric()
}

// ---- MustNewMultiResource cover ---------------------------------------

func TestMustNewMultiResourceHappy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := MustNewMultiResource(context.Background(), WithMegaAppName("must-mega"))
	if m == nil || m.appName != "must-mega" {
		t.Errorf("MustNewMultiResource result unexpected: %+v", m)
	}
}

func TestMustNewMultiResourcePanics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when appName missing")
		}
	}()
	_ = MustNewMultiResource(context.Background())
}

// ---- helpers (top-level test scope) ----------------------------------

func mustNewMultiTest(t *testing.T, name string) *MultiResource {
	t.Helper()
	m, err := NewMultiResource(context.Background(), WithMegaAppName(name))
	if err != nil {
		t.Fatalf("NewMultiResource(%q): %v", name, err)
	}
	return m
}

// Ensure subAppByPort delegates to subappForListener (sanity).
func TestSubAppByPortDelegates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := mustNewMultiTest(t, "mega")
	app := newMultiFakeApp("airdrop", 19890)
	if err := m.Register(app, OnPorts(PortMap{"l0": 19890})); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := m.subAppByPort(19890); got != "airdrop" {
		t.Errorf("subAppByPort = %q, want airdrop", got)
	}
	if got := m.subAppByPort(0); got != "" {
		t.Errorf("subAppByPort unknown port should return empty, got %q", got)
	}
}

// Compile-time guard: ensure multiFakeApp implements SubApp and sync is
// referenced (avoid dead-import slips during refactors).
var _ SubApp = (*multiFakeApp)(nil)
var _ = sync.Mutex{}
