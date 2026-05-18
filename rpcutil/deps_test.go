// deps_test.go — Wave 2 agent F white-box tests.
//
// Covers RegisterDB / DBFor / CacheFor / ValidateBudget.
// Uses newPoolFunc indirection to inject a fake pool factory, avoiding real PG.
package rpcutil

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stumble/wpgx"
)

// ---- helpers ----

func newDepsTestMR() *MultiResource {
	return &MultiResource{
		dbPools:   map[string]*wpgx.Pool{},
		dbConfigs: map[string]DBConfig{},
	}
}

// swapPoolFactory replaces newPoolFunc with fake; restored on cleanup.
func swapPoolFactory(t *testing.T, fake func(ctx context.Context, cfg *wpgx.Config) (*wpgx.Pool, error)) {
	t.Helper()
	orig := newPoolFunc
	newPoolFunc = fake
	t.Cleanup(func() { newPoolFunc = orig })
}

// fakePoolFactory returns (nil pool, nil err) and captures the cfg passed in.
// *wpgx.Pool==nil is a valid map value; closeFuncs are not exercised here
// because pool.Close() on nil would panic — that path is covered indirectly
// only by counting closeFuncs length, never by calling them.
func fakePoolFactory(captured **wpgx.Config) func(context.Context, *wpgx.Config) (*wpgx.Pool, error) {
	return func(_ context.Context, cfg *wpgx.Config) (*wpgx.Pool, error) {
		*captured = cfg
		return nil, nil
	}
}

// ---- A. RegisterDB: duplicate name ----

func TestRegisterDB_DuplicateName(t *testing.T) {
	m := newDepsTestMR()
	t.Setenv("DUP_USERNAME", "u")
	var got *wpgx.Config
	swapPoolFactory(t, fakePoolFactory(&got))

	if err := m.RegisterDB("primary", DBConfig{EnvPrefix: "DUP", MaxConns: 5}); err != nil {
		t.Fatalf("first RegisterDB: %v", err)
	}
	err := m.RegisterDB("primary", DBConfig{EnvPrefix: "DUP", MaxConns: 5})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

// ---- B. RegisterDB: missing RequiredEnv key ----

func TestRegisterDB_MissingRequiredEnv(t *testing.T) {
	m := newDepsTestMR()
	// Intentionally do NOT set REQ_PASSWORD / REQ_HOST.
	t.Setenv("REQ_USERNAME", "u")
	swapPoolFactory(t, func(context.Context, *wpgx.Config) (*wpgx.Pool, error) {
		t.Fatalf("pool factory should not be called when env missing")
		return nil, nil
	})

	err := m.RegisterDB("primary", DBConfig{
		EnvPrefix:   "REQ",
		RequiredEnv: []string{"PASSWORD", "HOST"},
	})
	if err == nil {
		t.Fatalf("expected missing env error")
	}
	if !strings.Contains(err.Error(), "REQ_PASSWORD") || !strings.Contains(err.Error(), "REQ_HOST") {
		t.Fatalf("error should list both missing keys, got %v", err)
	}
}

// ---- C. RegisterDB happy path + envconfig wiring ----

func TestRegisterDB_HappyPath_EnvconfigWiring(t *testing.T) {
	m := newDepsTestMR()
	t.Setenv("STAKING_USERNAME", "stake_user")
	t.Setenv("STAKING_PASSWORD", "stake_pw")
	t.Setenv("STAKING_HOST", "db.staking")
	t.Setenv("STAKING_PORT", "5433")
	t.Setenv("STAKING_DBNAME", "stake_db")
	t.Setenv("STAKING_APPNAME", "staking-app")

	var got *wpgx.Config
	swapPoolFactory(t, fakePoolFactory(&got))

	cfg := DBConfig{
		EnvPrefix:       "STAKING",
		MaxConns:        12,
		MinConns:        3,
		MaxConnLifetime: 30 * time.Minute,
		RequiredEnv:     []string{"USERNAME", "PASSWORD"},
	}
	if err := m.RegisterDB("staking", cfg); err != nil {
		t.Fatalf("RegisterDB: %v", err)
	}

	if got == nil {
		t.Fatalf("pool factory never called")
	}
	if got.Username != "stake_user" || got.Password != "stake_pw" {
		t.Errorf("envconfig username/password mismatch: %+v", got)
	}
	if got.Host != "db.staking" || got.Port != 5433 || got.DBName != "stake_db" {
		t.Errorf("envconfig host/port/dbname mismatch: %+v", got)
	}
	if got.MaxConns != 12 || got.MinConns != 3 {
		t.Errorf("user override MaxConns/MinConns lost: %+v", got)
	}
	if got.MaxConnLifetime != 30*time.Minute {
		t.Errorf("user override MaxConnLifetime lost: %v", got.MaxConnLifetime)
	}
	if got.AppName != "staking-app" {
		t.Errorf("AppName mismatch: %q", got.AppName)
	}

	if _, ok := m.dbConfigs["staking"]; !ok {
		t.Errorf("dbConfigs[\"staking\"] missing")
	}
	if _, ok := m.dbPools["staking"]; !ok {
		t.Errorf("dbPools[\"staking\"] missing")
	}
	if len(m.closeFuncs) != 1 {
		t.Errorf("expected 1 closeFunc, got %d", len(m.closeFuncs))
	}
}

// AppName fallback: when APPNAME env is empty, falls back to db pool name.
func TestRegisterDB_AppNameFallback(t *testing.T) {
	m := newDepsTestMR()
	t.Setenv("FB_USERNAME", "u")
	var got *wpgx.Config
	swapPoolFactory(t, fakePoolFactory(&got))

	if err := m.RegisterDB("primary", DBConfig{EnvPrefix: "FB"}); err != nil {
		t.Fatalf("RegisterDB: %v", err)
	}
	if got.AppName != "primary" {
		t.Errorf("expected AppName fallback to pool name, got %q", got.AppName)
	}
}

// "同实例不同库" 拓扑: 多 sub-app 共享同一 EnvPrefix (裸 POSTGRES_* 连接
// 凭证), 各自用 DBConfig.DBName/AppName 在代码里区分目标 database, 不依赖
// per-sub-app env 前缀。验证 override 生效且不互相污染。
func TestRegisterDB_SharedInstanceDBNameOverride(t *testing.T) {
	m := newDepsTestMR()
	// 共享实例凭证 (模拟 cnpg-shared 注入的裸 POSTGRES_*)。
	t.Setenv("POSTGRES_USERNAME", "galxe_app_user")
	t.Setenv("POSTGRES_PASSWORD", "shared_pw")
	t.Setenv("POSTGRES_HOST", "cnpg-shared-pooler-rw")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_DBNAME", "ignored_default") // 应被 cfg.DBName 覆盖

	var stakingCfg *wpgx.Config
	swapPoolFactory(t, fakePoolFactory(&stakingCfg))
	if err := m.RegisterDB("staking", DBConfig{
		EnvPrefix:   "POSTGRES",
		DBName:      "staking",
		AppName:     "galxe-mega-value-staking",
		MaxConns:    15,
		RequiredEnv: []string{"USERNAME", "PASSWORD", "HOST", "PORT"},
	}); err != nil {
		t.Fatalf("RegisterDB staking: %v", err)
	}

	var tokenCfg *wpgx.Config
	swapPoolFactory(t, fakePoolFactory(&tokenCfg))
	if err := m.RegisterDB("token", DBConfig{
		EnvPrefix:   "POSTGRES",
		DBName:      "token",
		AppName:     "galxe-mega-value-token",
		MaxConns:    10,
		RequiredEnv: []string{"USERNAME", "PASSWORD", "HOST", "PORT"},
	}); err != nil {
		t.Fatalf("RegisterDB token: %v", err)
	}

	// 共享凭证两边一致。
	if stakingCfg.Username != "galxe_app_user" || stakingCfg.Password != "shared_pw" ||
		stakingCfg.Host != "cnpg-shared-pooler-rw" || stakingCfg.Port != 5432 {
		t.Errorf("staking shared creds mismatch: %+v", stakingCfg)
	}
	if tokenCfg.Username != "galxe_app_user" || tokenCfg.Password != "shared_pw" {
		t.Errorf("token shared creds mismatch: %+v", tokenCfg)
	}
	// DBName/AppName 各自被 cfg 覆盖, 不互相污染, 且压过 env 默认值。
	if stakingCfg.DBName != "staking" || stakingCfg.AppName != "galxe-mega-value-staking" {
		t.Errorf("staking override lost: dbname=%q appname=%q", stakingCfg.DBName, stakingCfg.AppName)
	}
	if tokenCfg.DBName != "token" || tokenCfg.AppName != "galxe-mega-value-token" {
		t.Errorf("token override lost: dbname=%q appname=%q", tokenCfg.DBName, tokenCfg.AppName)
	}
	if stakingCfg.DBName == tokenCfg.DBName {
		t.Errorf("per-sub-app DBName not isolated: both %q", stakingCfg.DBName)
	}
}

// Empty DBName/AppName -> no override, envconfig value preserved.
func TestRegisterDB_EmptyDBNameNoOverride(t *testing.T) {
	m := newDepsTestMR()
	t.Setenv("NOOV_USERNAME", "u")
	t.Setenv("NOOV_DBNAME", "from_env")
	t.Setenv("NOOV_APPNAME", "from_env_app")
	var got *wpgx.Config
	swapPoolFactory(t, fakePoolFactory(&got))

	if err := m.RegisterDB("primary", DBConfig{EnvPrefix: "NOOV"}); err != nil {
		t.Fatalf("RegisterDB: %v", err)
	}
	if got.DBName != "from_env" {
		t.Errorf("empty cfg.DBName must not override env, got %q", got.DBName)
	}
	if got.AppName != "from_env_app" {
		t.Errorf("empty cfg.AppName must not override env, got %q", got.AppName)
	}
}

// Empty name / EnvPrefix -> error.
func TestRegisterDB_RejectsEmptyArgs(t *testing.T) {
	m := newDepsTestMR()
	if err := m.RegisterDB("", DBConfig{EnvPrefix: "X"}); err == nil {
		t.Errorf("expected error on empty name")
	}
	if err := m.RegisterDB("ok", DBConfig{EnvPrefix: ""}); err == nil {
		t.Errorf("expected error on empty EnvPrefix")
	}
}

// Pool factory returns error -> bubbled up, nothing stored.
func TestRegisterDB_FactoryError(t *testing.T) {
	m := newDepsTestMR()
	t.Setenv("ERR_USERNAME", "u")
	bang := errors.New("boom")
	swapPoolFactory(t, func(context.Context, *wpgx.Config) (*wpgx.Pool, error) {
		return nil, bang
	})

	err := m.RegisterDB("p", DBConfig{EnvPrefix: "ERR"})
	if err == nil || !errors.Is(err, bang) {
		t.Fatalf("expected wrapped factory err, got %v", err)
	}
	if len(m.dbConfigs) != 0 || len(m.dbPools) != 0 {
		t.Errorf("nothing should be stored on factory error")
	}
}

// ---- D. DBFor ----

func TestDBFor(t *testing.T) {
	m := newDepsTestMR()
	t.Setenv("OK_USERNAME", "u")
	var got *wpgx.Config
	swapPoolFactory(t, fakePoolFactory(&got))

	if err := m.RegisterDB("p", DBConfig{EnvPrefix: "OK"}); err != nil {
		t.Fatalf("RegisterDB: %v", err)
	}

	if _, err := m.DBFor("p"); err != nil {
		t.Errorf("DBFor(registered) unexpected err: %v", err)
	}
	_, err := m.DBFor("nope")
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Errorf("DBFor(unknown) expected error, got %v", err)
	}
}

// ---- E. CacheFor returns prefix wrapper ----

func TestCacheFor_WrapperHasPrefix(t *testing.T) {
	m := newDepsTestMR()
	c := m.CacheFor("airdrop")
	sc, ok := c.(*subappCache)
	if !ok {
		t.Fatalf("expected *subappCache, got %T", c)
	}
	if sc.prefix != "airdrop:" {
		t.Errorf("prefix = %q, want %q", sc.prefix, "airdrop:")
	}
	if sc.underlying != nil {
		t.Errorf("expected nil underlying when m.dcache==nil")
	}

	// nil underlying -> every method returns error (no panic).
	ctx := context.Background()
	if err := sc.Set(ctx, "k", "v", time.Second); err == nil {
		t.Errorf("Set on nil underlying should error")
	}
	if err := sc.Invalidate(ctx, "k"); err == nil {
		t.Errorf("Invalidate on nil underlying should error")
	}
	if err := sc.Ping(ctx); err == nil {
		t.Errorf("Ping on nil underlying should error")
	}
	if err := sc.Get(ctx, "k", nil, time.Second, nil, false, false); err == nil {
		t.Errorf("Get on nil underlying should error")
	}
	if err := sc.GetWithTtl(ctx, "k", nil, nil, false, false); err == nil {
		t.Errorf("GetWithTtl on nil underlying should error")
	}
}

// Verify the key prefixing math directly.
func TestSubappCache_KeyPrefixing(t *testing.T) {
	sc := &subappCache{prefix: "airdrop:"}
	if got := sc.key("user:42"); got != "airdrop:user:42" {
		t.Errorf("key prefixing wrong: %q", got)
	}
	if got := sc.key(""); got != "airdrop:" {
		t.Errorf("empty key prefix wrong: %q", got)
	}
}

// ---- F/G/H/I. ValidateBudget ----

func TestValidateBudget_WithinBudget(t *testing.T) {
	m := newDepsTestMR()
	m.dbConfigs["a"] = DBConfig{MaxConns: 10}
	m.dbConfigs["b"] = DBConfig{MaxConns: 15}
	// 25 * 4 + 50 = 150 ; (500 - 20) * 0.8 = 384 -> OK
	err := m.ValidateBudget(4, BudgetConfig{
		InstanceMaxConnections: 500,
		SuperuserReserved:      20,
		SafetyFactor:           0.8,
		AdminPoolReserved:      50,
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateBudget_Exceeds(t *testing.T) {
	m := newDepsTestMR()
	m.dbConfigs["a"] = DBConfig{MaxConns: 50}
	m.dbConfigs["b"] = DBConfig{MaxConns: 50}
	// 100 * 5 + 0 = 500 ; (200-10)*0.8 = 152 -> exceeds
	err := m.ValidateBudget(5, BudgetConfig{
		InstanceMaxConnections: 200,
		SuperuserReserved:      10,
		SafetyFactor:           0.8,
		AdminPoolReserved:      0,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{"needed", "available", "exceeded"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q: %s", want, msg)
		}
	}
}

func TestValidateBudget_MaxReplicasInvalid(t *testing.T) {
	m := newDepsTestMR()
	for _, mr := range []int{0, -1, -100} {
		if err := m.ValidateBudget(mr, BudgetConfig{InstanceMaxConnections: 100, SafetyFactor: 0.8}); err == nil {
			t.Errorf("expected error for maxReplicas=%d", mr)
		}
	}
}

func TestValidateBudget_SafetyFactorInvalid(t *testing.T) {
	m := newDepsTestMR()
	if err := m.ValidateBudget(1, BudgetConfig{InstanceMaxConnections: 100, SafetyFactor: 0}); err == nil {
		t.Errorf("expected error for SafetyFactor=0")
	}
}

func TestValidateBudget_NoDBsRegistered(t *testing.T) {
	m := newDepsTestMR()
	// sum=0; used = 0 + admin = 5 ; budget = (100-0)*0.5 = 50 -> OK.
	err := m.ValidateBudget(3, BudgetConfig{
		InstanceMaxConnections: 100,
		SafetyFactor:           0.5,
		AdminPoolReserved:      5,
	})
	if err != nil {
		t.Errorf("expected nil for empty dbConfigs, got %v", err)
	}
}

// AdminPoolReserved alone can exceed the budget.
func TestValidateBudget_AdminAloneExceeds(t *testing.T) {
	m := newDepsTestMR()
	// 0 * 1 + 100 = 100 ; (50-0)*0.8 = 40 -> exceeds.
	err := m.ValidateBudget(1, BudgetConfig{
		InstanceMaxConnections: 50,
		SafetyFactor:           0.8,
		AdminPoolReserved:      100,
	})
	if err == nil {
		t.Fatal("expected exceed error from admin alone")
	}
}

// ---- J. Concurrency: RegisterDB + DBFor on different names ----

func TestRegisterDB_ConcurrentDifferentNames(t *testing.T) {
	m := newDepsTestMR()
	t.Setenv("CON_USERNAME", "u")
	var counter int32
	swapPoolFactory(t, func(_ context.Context, _ *wpgx.Config) (*wpgx.Pool, error) {
		atomic.AddInt32(&counter, 1)
		return nil, nil
	})

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n*2)
	for i := 0; i < n; i++ {
		wg.Add(2)
		name := "pool_" + string(rune('a'+i))
		go func(name string) {
			defer wg.Done()
			if err := m.RegisterDB(name, DBConfig{EnvPrefix: "CON"}); err != nil {
				errs <- err
			}
		}(name)
		go func(name string) {
			defer wg.Done()
			// Either nil (found) or "not registered" — both race-safe.
			_, _ = m.DBFor(name)
		}(name)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Errorf("concurrent RegisterDB err: %v", e)
	}
	if atomic.LoadInt32(&counter) != n {
		t.Errorf("expected %d pool factory calls, got %d", n, atomic.LoadInt32(&counter))
	}
	if len(m.dbConfigs) != n {
		t.Errorf("expected %d dbConfigs, got %d", n, len(m.dbConfigs))
	}
}

// Concurrent duplicate-name RegisterDB: at most one wins.
func TestRegisterDB_ConcurrentDuplicate(t *testing.T) {
	m := newDepsTestMR()
	t.Setenv("DUPC_USERNAME", "u")
	var calls int32
	swapPoolFactory(t, func(context.Context, *wpgx.Config) (*wpgx.Pool, error) {
		atomic.AddInt32(&calls, 1)
		return nil, nil
	})

	const n = 10
	var wg sync.WaitGroup
	var okCount int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.RegisterDB("same", DBConfig{EnvPrefix: "DUPC"}); err == nil {
				atomic.AddInt32(&okCount, 1)
			}
		}()
	}
	wg.Wait()
	if okCount != 1 {
		t.Errorf("expected exactly 1 success, got %d", okCount)
	}
}
