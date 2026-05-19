// Tests for the subapp ctx hook (mega framework L1).
//
// Black-box tests: exercise WithSubApp / SubAppFromCtx purely through the
// exported API, as a downstream consumer would.
package zerolog_test

import (
	"context"
	"sync"
	"testing"

	"github.com/ggsrc/gopkg/zerolog"
)

// foreignKey is a test-local context key type used to verify that
// SubAppFromCtx does NOT leak values stored under unrelated keys.
type foreignKey struct{}

func TestSubAppFromCtx_EmptyCtx(t *testing.T) {
	t.Parallel()

	got, ok := zerolog.SubAppFromCtx(context.Background())
	if ok {
		t.Fatalf("ok = true on background ctx; want false")
	}
	if got != "" {
		t.Fatalf("got = %q on background ctx; want empty string", got)
	}
}

func TestWithSubApp_RoundTrip(t *testing.T) {
	t.Parallel()

	const want = "galxe-airdrop"
	ctx := zerolog.WithSubApp(context.Background(), want)

	got, ok := zerolog.SubAppFromCtx(ctx)
	if !ok {
		t.Fatalf("ok = false after WithSubApp; want true")
	}
	if got != want {
		t.Fatalf("got = %q; want %q", got, want)
	}
}

func TestWithSubApp_NestedInnermostWins(t *testing.T) {
	t.Parallel()

	ctx := zerolog.WithSubApp(context.Background(), "outer")
	ctx = zerolog.WithSubApp(ctx, "inner")

	got, ok := zerolog.SubAppFromCtx(ctx)
	if !ok {
		t.Fatalf("ok = false after nested WithSubApp; want true")
	}
	if got != "inner" {
		t.Fatalf("got = %q; want %q (innermost wins)", got, "inner")
	}
}

func TestSubAppFromCtx_TypeIsolation(t *testing.T) {
	t.Parallel()

	// A ctx that has a string value at a totally different key type must
	// not be visible through SubAppFromCtx — the unexported subappCtxKey
	// type is the only valid lookup key.
	ctx := context.WithValue(context.Background(), foreignKey{}, "not-a-subapp")

	got, ok := zerolog.SubAppFromCtx(ctx)
	if ok {
		t.Fatalf("ok = true with only foreign key set; want false (got value %q)", got)
	}
	if got != "" {
		t.Fatalf("got = %q with only foreign key set; want empty string", got)
	}
}

// TestWithSubApp_EmptyString documents intentional behavior: WithSubApp
// stores whatever string is given, including "". Presence in ctx is
// distinct from non-empty content, so SubAppFromCtx returns ("", true).
// Callers that want "missing OR empty" semantics must check the string
// themselves.
func TestWithSubApp_EmptyString(t *testing.T) {
	t.Parallel()

	ctx := zerolog.WithSubApp(context.Background(), "")

	got, ok := zerolog.SubAppFromCtx(ctx)
	if !ok {
		t.Fatalf("ok = false after WithSubApp(ctx, \"\"); want true (presence != non-empty)")
	}
	if got != "" {
		t.Fatalf("got = %q; want empty string", got)
	}
}

// TestSubAppFromCtx_ConcurrentReads verifies that a ctx produced by
// WithSubApp is safe to read concurrently from many goroutines. Run with
// -race to catch any accidental shared mutable state.
func TestSubAppFromCtx_ConcurrentReads(t *testing.T) {
	t.Parallel()

	const (
		want       = "galxe-campaign"
		goroutines = 64
		reads      = 1000
	)

	ctx := zerolog.WithSubApp(context.Background(), want)

	var wg sync.WaitGroup
	mismatches := make(chan string, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < reads; j++ {
				got, ok := zerolog.SubAppFromCtx(ctx)
				if !ok || got != want {
					select {
					case mismatches <- got:
					default:
					}
					return
				}
			}
		}()
	}

	wg.Wait()
	close(mismatches)

	for got := range mismatches {
		t.Fatalf("concurrent read mismatch: got = %q; want %q", got, want)
	}
}

// TestSubAppFromCtx_TableDriven covers a handful of common values in
// one place; useful as a regression net should the storage mechanism
// ever change.
func TestSubAppFromCtx_TableDriven(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		subapp string
	}{
		{name: "simple", subapp: "airdrop"},
		{name: "hyphenated", subapp: "galxe-loyalty-points"},
		{name: "with-dots", subapp: "team.svc.v2"},
		{name: "unicode", subapp: "子应用-α"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := zerolog.WithSubApp(context.Background(), tc.subapp)
			got, ok := zerolog.SubAppFromCtx(ctx)
			if !ok {
				t.Fatalf("ok = false; want true")
			}
			if got != tc.subapp {
				t.Fatalf("got = %q; want %q", got, tc.subapp)
			}
		})
	}
}
