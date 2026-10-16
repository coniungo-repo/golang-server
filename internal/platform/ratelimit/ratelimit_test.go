package ratelimit

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
)

var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func allowN(l *Limiter, key string, n int) int {
	allowed := 0
	for range n {
		if ok, _ := l.Allow(key); ok {
			allowed++
		}
	}
	return allowed
}

func TestAllowBurstThenRefill(t *testing.T) {
	c := clock.NewMock(start)
	l := New(c, []config.RateWindow{{Limit: 10, Period: time.Minute}})

	if got := allowN(l, "a", 15); got != 10 {
		t.Fatalf("burst allowed %d, want 10", got)
	}
	ok, wait := l.Allow("a")
	if ok || wait != 6*time.Second {
		t.Fatalf("got ok=%v wait=%s, want refusal for 6s", ok, wait)
	}
	c.Add(6 * time.Second)
	if got := allowN(l, "a", 2); got != 1 {
		t.Fatalf("after one interval allowed %d, want 1", got)
	}
	c.Add(time.Minute)
	if got := allowN(l, "a", 15); got != 10 {
		t.Fatalf("after a full period allowed %d, want 10", got)
	}
}

func TestRefusalsAreNotCounted(t *testing.T) {
	c := clock.NewMock(start)
	l := New(c, []config.RateWindow{{Limit: 2, Period: time.Second}})
	allowN(l, "a", 100)
	c.Add(500 * time.Millisecond)
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("refused requests must not push the limit further out")
	}
}

func TestEveryWindowApplies(t *testing.T) {
	c := clock.NewMock(start)
	l := New(c, []config.RateWindow{{Limit: 5, Period: time.Second}, {Limit: 8, Period: time.Hour}})
	if got := allowN(l, "a", 10); got != 5 {
		t.Fatalf("allowed %d, want 5 from the short window", got)
	}
	c.Add(time.Second)
	if got := allowN(l, "a", 10); got != 3 {
		t.Fatalf("allowed %d, want the 3 left in the hour window", got)
	}
	if _, wait := l.Allow("a"); wait < 7*time.Minute {
		t.Fatalf("wait %s should come from the hour window", wait)
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := New(clock.NewMock(start), []config.RateWindow{{Limit: 1, Period: time.Minute}})
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("first request refused")
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("another key was limited")
	}
}

func TestNoWindowsAllowsEverything(t *testing.T) {
	var nilLimiter *Limiter
	for _, l := range []*Limiter{nilLimiter, New(clock.New(), nil)} {
		if got := allowN(l, "a", 1000); got != 1000 {
			t.Fatalf("allowed %d, want all", got)
		}
	}
}

func TestSweep(t *testing.T) {
	c := clock.NewMock(start)
	l := New(c, []config.RateWindow{{Limit: 10, Period: time.Minute}})
	allowN(l, "busy", 10)
	allowN(l, "idle", 1)
	c.Add(6 * time.Second)
	if n := l.Sweep(); n != 1 {
		t.Fatalf("swept %d, want only the recovered key", n)
	}
	if got := allowN(l, "busy", 10); got != 1 {
		t.Fatalf("busy key allowed %d after sweep, want 1: its state must be kept", got)
	}
}

func TestConcurrentAllow(t *testing.T) {
	l := New(clock.NewMock(start), []config.RateWindow{{Limit: 100, Period: time.Hour}})
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			for range 20 {
				if ok, _ := l.Allow("shared"); ok {
					allowed.Add(1)
				}
				l.Allow(fmt.Sprint("own-", i))
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 100 {
		t.Fatalf("allowed %d, want exactly 100", allowed.Load())
	}
}

func TestClientKey(t *testing.T) {
	tests := map[string]string{
		"203.0.113.7":              "203.0.113.7",
		"2001:db8:1:2:3:4:5:6":     "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::1":     "2001:db8:1:2::/64",
		"not-an-ip":                "not-an-ip",
		"2001:db8:1:3:3:4:5:6%en0": "2001:db8:1:3::/64",
	}
	for in, want := range tests {
		if got := ClientKey(in); got != want {
			t.Errorf("ClientKey(%q) = %q, want %q", in, got, want)
		}
	}
}
