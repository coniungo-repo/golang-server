package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type pinger struct{ err error }

func (p pinger) Ping(context.Context) error { return p.err }

func probeStatus(t *testing.T, p *Probe, path string) int {
	t.Helper()
	mux := http.NewServeMux()
	p.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	return rr.Code
}

func TestProbe(t *testing.T) {
	healthy := New(pinger{})
	if probeStatus(t, healthy, "/livez") != 200 || probeStatus(t, healthy, "/readyz") != 200 {
		t.Fatal("healthy probe should pass")
	}

	down := New(pinger{err: errors.New("down")})
	if probeStatus(t, down, "/livez") != 200 || probeStatus(t, down, "/readyz") != 503 {
		t.Fatal("database outage should fail readiness only")
	}

	healthy.Drain()
	if probeStatus(t, healthy, "/readyz") != 503 {
		t.Fatal("draining probe should fail readiness")
	}
}

type countingPinger struct {
	calls atomic.Int64
	err   error
}

func (p *countingPinger) Ping(context.Context) error {
	p.calls.Add(1)
	time.Sleep(5 * time.Millisecond)
	return p.err
}

func TestReadyzSharesThePing(t *testing.T) {
	db := &countingPinger{}
	p := New(db)
	now := time.Unix(0, 0)
	var mu sync.Mutex
	p.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if got := probeStatus(t, p, "/readyz"); got != 200 {
				t.Errorf("status %d", got)
			}
		})
	}
	wg.Wait()
	if n := db.calls.Load(); n != 1 {
		t.Fatalf("50 concurrent probes pinged %d times, want 1", n)
	}

	mu.Lock()
	now = now.Add(pingTTL)
	mu.Unlock()
	db.err = errors.New("down")
	if got := probeStatus(t, p, "/readyz"); got != 503 || db.calls.Load() != 2 {
		t.Fatalf("after the ttl: status %d, pings %d; want a fresh failing ping", got, db.calls.Load())
	}
}
