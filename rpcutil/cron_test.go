package rpcutil

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog"
)

// mockScheduler is a test double for the Scheduler interface.
type mockScheduler struct {
	mu       sync.Mutex
	jobs     map[string]func() // fullName → wrapped fn
	schedule map[string]string // fullName → schedule
	regErr   error
	started  bool
	stopped  bool
}

func newMockScheduler() *mockScheduler {
	return &mockScheduler{
		jobs:     map[string]func(){},
		schedule: map[string]string{},
	}
}

func (m *mockScheduler) RegisterCronJob(name, schedule string, fn func()) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.regErr != nil {
		return m.regErr
	}
	m.jobs[name] = fn
	m.schedule[name] = schedule
	return nil
}

func (m *mockScheduler) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
}

func (m *mockScheduler) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	return nil
}

func (m *mockScheduler) jobCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.jobs)
}

func (m *mockScheduler) getJob(name string) (func(), bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn, ok := m.jobs[name]
	return fn, ok
}

// newCronTestMultiResource builds a MultiResource suitable for cron tests.
func newCronTestMultiResource(sched Scheduler, cronDisabled bool) *MultiResource {
	logger := zerolog.Nop()
	return &MultiResource{
		appName:      "test",
		rootLogger:   &logger,
		scheduler:    sched,
		cronDisabled: cronDisabled,
		cronJobs:     map[string]bool{},
	}
}

// histogramSampleCount returns the cumulative sample count for the
// cronDurationHist series with the given labels.
func histogramSampleCount(t *testing.T, subapp, name string) uint64 {
	t.Helper()
	obs := cronDurationHist.WithLabelValues(subapp, name)
	metric, ok := obs.(prometheus.Metric)
	if !ok {
		t.Fatalf("observer is not a prometheus.Metric")
	}
	pb := &dto.Metric{}
	if err := metric.Write(pb); err != nil {
		t.Fatalf("metric.Write: %v", err)
	}
	if pb.Histogram == nil {
		return 0
	}
	return pb.Histogram.GetSampleCount()
}

// --- registerCron tests ---

func TestRegisterCronSuccess(t *testing.T) {
	sched := newMockScheduler()
	m := newCronTestMultiResource(sched, false)

	called := atomic.Int32{}
	jobs := []CronJob{
		{
			Name:     "daily",
			Schedule: "0 0 * * *",
			Fn: func(ctx context.Context) error {
				called.Add(1)
				return nil
			},
		},
	}

	if err := m.registerCron("airdrop", jobs); err != nil {
		t.Fatalf("registerCron err: %v", err)
	}

	if sched.jobCount() != 1 {
		t.Fatalf("expected 1 job registered, got %d", sched.jobCount())
	}
	fn, ok := sched.getJob("airdrop.daily")
	if !ok {
		t.Fatalf("expected job airdrop.daily to be registered")
	}
	if sched.schedule["airdrop.daily"] != "0 0 * * *" {
		t.Fatalf("schedule string mismatch: %q", sched.schedule["airdrop.daily"])
	}
	if !m.cronJobs["airdrop.daily"] {
		t.Fatalf("expected cronJobs map to contain airdrop.daily")
	}
	// Invoke the wrapped fn → underlying Fn should run.
	fn()
	if called.Load() != 1 {
		t.Fatalf("expected underlying fn to be invoked once, got %d", called.Load())
	}
}

func TestRegisterCronDuplicateWithinSlice(t *testing.T) {
	sched := newMockScheduler()
	m := newCronTestMultiResource(sched, false)

	jobs := []CronJob{
		{Name: "dupA", Schedule: "* * * * *", Fn: func(ctx context.Context) error { return nil }},
		{Name: "dupA", Schedule: "* * * * *", Fn: func(ctx context.Context) error { return nil }},
	}
	err := m.registerCron("subdup", jobs)
	if err == nil {
		t.Fatalf("expected duplicate error, got nil")
	}
}

func TestRegisterCronDuplicateAcrossCalls(t *testing.T) {
	sched := newMockScheduler()
	m := newCronTestMultiResource(sched, false)

	job := CronJob{Name: "dupB", Schedule: "* * * * *", Fn: func(ctx context.Context) error { return nil }}
	if err := m.registerCron("subdup2", []CronJob{job}); err != nil {
		t.Fatalf("first registerCron failed: %v", err)
	}
	err := m.registerCron("subdup2", []CronJob{job})
	if err == nil {
		t.Fatalf("expected duplicate error on second call, got nil")
	}
}

func TestRegisterCronDisabled(t *testing.T) {
	sched := newMockScheduler()
	m := newCronTestMultiResource(sched, true)

	jobs := []CronJob{
		{Name: "j1", Schedule: "* * * * *", Fn: func(ctx context.Context) error { return nil }},
	}
	if err := m.registerCron("subdis", jobs); err != nil {
		t.Fatalf("registerCron err: %v", err)
	}
	if sched.jobCount() != 0 {
		t.Fatalf("expected scheduler NOT called when disabled, got %d jobs", sched.jobCount())
	}
	if len(m.cronJobs) != 0 {
		t.Fatalf("expected cronJobs map empty when disabled, got %d", len(m.cronJobs))
	}
}

func TestRegisterCronSchedulerError(t *testing.T) {
	sched := newMockScheduler()
	sched.regErr = errors.New("boom")
	m := newCronTestMultiResource(sched, false)

	jobs := []CronJob{
		{Name: "j", Schedule: "* * * * *", Fn: func(ctx context.Context) error { return nil }},
	}
	err := m.registerCron("suberr", jobs)
	if err == nil {
		t.Fatalf("expected error from scheduler")
	}
	if !errors.Is(err, sched.regErr) {
		t.Fatalf("expected wrapped scheduler error, got %v", err)
	}
}

func TestRegisterCronLazyInitMap(t *testing.T) {
	sched := newMockScheduler()
	logger := zerolog.Nop()
	m := &MultiResource{
		appName:    "test",
		rootLogger: &logger,
		scheduler:  sched,
		cronJobs:   nil, // explicitly nil
	}
	jobs := []CronJob{
		{Name: "lz", Schedule: "* * * * *", Fn: func(ctx context.Context) error { return nil }},
	}
	if err := m.registerCron("sublz", jobs); err != nil {
		t.Fatalf("registerCron err: %v", err)
	}
	if !m.cronJobs["sublz.lz"] {
		t.Fatalf("expected lazy-init cronJobs map to contain sublz.lz")
	}
}

// --- wrapCronFn tests ---

func TestWrapCronFnPanicRecovered(t *testing.T) {
	m := newCronTestMultiResource(newMockScheduler(), false)
	subapp := "subPanic"
	name := "subPanic.j"

	before := testutil.ToFloat64(cronPanicCounter.WithLabelValues(subapp, name))

	wrapped := m.wrapCronFn(subapp, name, func(ctx context.Context) error {
		panic("kaboom")
	})

	// Must not propagate panic.
	wrapped()

	after := testutil.ToFloat64(cronPanicCounter.WithLabelValues(subapp, name))
	if after-before != 1 {
		t.Fatalf("expected panic counter +1, got delta %v", after-before)
	}
}

func TestWrapCronFnErrorCounted(t *testing.T) {
	m := newCronTestMultiResource(newMockScheduler(), false)
	subapp := "subErr"
	name := "subErr.j"

	before := testutil.ToFloat64(cronErrorCounter.WithLabelValues(subapp, name))

	wrapped := m.wrapCronFn(subapp, name, func(ctx context.Context) error {
		return errors.New("nope")
	})
	wrapped()

	after := testutil.ToFloat64(cronErrorCounter.WithLabelValues(subapp, name))
	if after-before != 1 {
		t.Fatalf("expected error counter +1, got delta %v", after-before)
	}
}

func TestWrapCronFnSuccessObservesDuration(t *testing.T) {
	m := newCronTestMultiResource(newMockScheduler(), false)
	subapp := "subOk"
	name := "subOk.j"

	beforeErr := testutil.ToFloat64(cronErrorCounter.WithLabelValues(subapp, name))
	beforePanic := testutil.ToFloat64(cronPanicCounter.WithLabelValues(subapp, name))
	beforeCount := histogramSampleCount(t, subapp, name)

	wrapped := m.wrapCronFn(subapp, name, func(ctx context.Context) error {
		return nil
	})
	wrapped()

	afterErr := testutil.ToFloat64(cronErrorCounter.WithLabelValues(subapp, name))
	afterPanic := testutil.ToFloat64(cronPanicCounter.WithLabelValues(subapp, name))
	afterCount := histogramSampleCount(t, subapp, name)

	if afterErr != beforeErr {
		t.Fatalf("expected error counter unchanged on success, got delta %v", afterErr-beforeErr)
	}
	if afterPanic != beforePanic {
		t.Fatalf("expected panic counter unchanged on success, got delta %v", afterPanic-beforePanic)
	}
	if afterCount-beforeCount != 1 {
		t.Fatalf("expected duration histogram +1 sample, got delta %d", afterCount-beforeCount)
	}
}

func TestWrapCronFnPassesNonNilCtx(t *testing.T) {
	m := newCronTestMultiResource(newMockScheduler(), false)
	var seen context.Context
	wrapped := m.wrapCronFn("subapp", "subapp.j", func(ctx context.Context) error {
		seen = ctx
		return nil
	})
	wrapped()
	if seen == nil {
		t.Fatalf("expected non-nil ctx passed to fn")
	}
}

// --- default scheduler tests ---

func TestNewDefaultScheduler(t *testing.T) {
	s, err := newDefaultScheduler()
	if err != nil {
		t.Fatalf("newDefaultScheduler err: %v", err)
	}
	if s == nil {
		t.Fatalf("expected non-nil scheduler")
	}
	if err := s.RegisterCronJob("test.j", "* * * * *", func() {}); err != nil {
		t.Fatalf("RegisterCronJob err: %v", err)
	}
	// Start + Stop should not panic.
	s.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatalf("Stop err: %v", err)
	}
}

func TestNewDefaultSchedulerInvalidCron(t *testing.T) {
	s, err := newDefaultScheduler()
	if err != nil {
		t.Fatalf("newDefaultScheduler err: %v", err)
	}
	defer func() {
		_ = s.Stop(context.Background())
	}()
	err = s.RegisterCronJob("bad", "not a cron expr", func() {})
	if err == nil {
		t.Fatalf("expected error for invalid cron expression")
	}
}

func TestDefaultSchedulerStopReturnsPromptly(t *testing.T) {
	s, err := newDefaultScheduler()
	if err != nil {
		t.Fatalf("newDefaultScheduler err: %v", err)
	}
	s.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Stop(ctx) }()
	select {
	case <-done:
		// OK — Stop returned.
	case <-time.After(5 * time.Second):
		t.Fatalf("Stop did not return promptly")
	}
}
