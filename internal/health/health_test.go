package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
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
