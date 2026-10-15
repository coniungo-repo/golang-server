package auth

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testRetention = 24 * time.Hour

// testLimiter needs TEST_DATABASE_URL pointing at a migrated database; the test is skipped without it.
func testLimiter(t *testing.T) (*Limiter, *clock.Mock) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	clk := clock.NewMock(time.Now().Add(-48 * time.Hour).Truncate(time.Second))
	secret := t.Name() + time.Now().String()
	return NewLimiter(pool, secret, clk, testRetention), clk
}

func TestLimiterWindows(t *testing.T) {
	l, clk := testLimiter(t)
	ctx := context.Background()
	policy := Policy{Kind: "test", Windows: []config.RateWindow{{Limit: 2, Period: time.Minute}, {Limit: 3, Period: time.Hour}}}
	try := func() bool {
		ok, err := l.Allow(ctx, Attempt{Policy: policy, Value: "phone:+2348031234567"})
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}

	for i := range 2 {
		if !try() {
			t.Fatalf("attempt %d should pass", i+1)
		}
	}
	if try() {
		t.Fatal("third attempt inside a minute should be blocked")
	}
	clk.Add(2 * time.Minute)
	if !try() {
		t.Fatal("attempt after the short window should pass")
	}
	clk.Add(2 * time.Minute)
	if try() {
		t.Fatal("fourth attempt inside an hour should be blocked by the long window")
	}
	clk.Add(time.Hour)
	if !try() {
		t.Fatal("attempt after the long window should pass")
	}
}

func TestLimiterBlockedAttemptRecordsNothing(t *testing.T) {
	l, _ := testLimiter(t)
	ctx := context.Background()
	tight := Policy{Kind: "tight", Windows: []config.RateWindow{{Limit: 1, Period: time.Hour}}}
	loose := Policy{Kind: "loose", Windows: []config.RateWindow{{Limit: 2, Period: time.Hour}}}

	if ok, _ := l.Allow(ctx, Attempt{tight, "a"}, Attempt{loose, "ip"}); !ok {
		t.Fatal("first should pass")
	}
	if ok, _ := l.Allow(ctx, Attempt{tight, "a"}, Attempt{loose, "ip"}); ok {
		t.Fatal("second should be blocked by tight")
	}
	if ok, _ := l.Allow(ctx, Attempt{tight, "b"}, Attempt{loose, "ip"}); !ok {
		t.Fatal("blocked attempt must not have used up the loose budget")
	}
}

func TestLimiterConcurrent(t *testing.T) {
	l, _ := testLimiter(t)
	policy := Policy{Kind: "burst", Windows: []config.RateWindow{{Limit: 5, Period: time.Hour}}}
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for range 40 {
		wg.Go(func() {
			ok, err := l.Allow(context.Background(), Attempt{policy, "phone:+2348000000000"})
			if err != nil {
				t.Error(err)
			}
			if ok {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 5 {
		t.Fatalf("allowed %d of 40 concurrent attempts, want exactly 5", allowed.Load())
	}
}

func TestLimiterPrune(t *testing.T) {
	l, clk := testLimiter(t)
	ctx := context.Background()
	if _, err := l.Allow(ctx, Attempt{Policy{Kind: "prune", Windows: []config.RateWindow{{Limit: 9, Period: time.Hour}}}, "x"}); err != nil {
		t.Fatal(err)
	}
	clk.Add(testRetention + time.Minute)
	n, err := l.Prune(ctx)
	if err != nil || n < 1 {
		t.Fatalf("pruned %d, err %v", n, err)
	}
}
