package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/metrics"
	"github.com/coniungo-repo/golang-server/internal/platform/ratelimit"
	"github.com/coniungo-repo/golang-server/internal/platform/respond"
)

func TestRateLimit(t *testing.T) {
	l := ratelimit.New(clock.NewMock(time.Unix(0, 0)), []config.RateWindow{{Limit: 2, Period: time.Minute}})
	m := metrics.New("test")
	h := RateLimit("test", l, func(r *http.Request) string { return r.Header.Get("X-Key") }, m)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	do := func(key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Key", key)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		return rr
	}

	for range 2 {
		if rr := do("a"); rr.Code != http.StatusNoContent {
			t.Fatalf("status %d within the limit", rr.Code)
		}
	}
	rr := do("a")
	if rr.Code != http.StatusTooManyRequests || rr.Header().Get("Retry-After") != "30" || rr.Header().Get("Content-Type") != respond.ContentTypeJSON {
		t.Fatalf("got %d, Retry-After %q, Content-Type %q", rr.Code, rr.Header().Get("Retry-After"), rr.Header().Get("Content-Type"))
	}
	if !strings.Contains(scrapeMetrics(t, m), `rate_limited_total{limit="test"} 1`) {
		t.Fatal("refusal not counted")
	}
	if rr := do("b"); rr.Code != http.StatusNoContent {
		t.Fatalf("another key got %d", rr.Code)
	}
	for range 5 {
		if rr := do(""); rr.Code != http.StatusNoContent {
			t.Fatalf("an empty key must not be limited, got %d", rr.Code)
		}
	}
}
