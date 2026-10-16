package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/internal/health"
	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/ratelimit"
	"github.com/coniungo-repo/golang-server/internal/platform/requestid"
)

type okPinger struct{}

func (okPinger) Ping(context.Context) error { return nil }

func testRouter(t *testing.T) http.Handler {
	t.Helper()
	var cfg config.Config
	cfg.Server.RequestTimeout = time.Second
	h, err := newRouter(&dependencies{Config: cfg, Health: health.New(okPinger{}, time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestRouter(t *testing.T) {
	h := testRouter(t)
	tests := []struct {
		method, path string
		want         int
		allow        string
	}{
		{http.MethodGet, "/livez", 200, ""},
		{http.MethodGet, "/readyz", 200, ""},
		{http.MethodPost, "/livez", 405, "GET"},
		{http.MethodGet, "/nope", 404, ""},
		{http.MethodGet, "/api/v1/nope", 404, ""},
	}
	for _, tt := range tests {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(tt.method, tt.path, nil))
		if rr.Code != tt.want {
			t.Fatalf("%s %s: status %d, want %d", tt.method, tt.path, rr.Code, tt.want)
		}
		if rr.Header().Get("Allow") != tt.allow {
			t.Fatalf("%s %s: Allow %q, want %q", tt.method, tt.path, rr.Header().Get("Allow"), tt.allow)
		}
		if rr.Header().Get(requestid.Header) == "" || rr.Header().Get("X-Content-Type-Options") == "" {
			t.Fatalf("%s %s: middleware not applied", tt.method, tt.path)
		}
		if rr.Code >= 400 && rr.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatalf("%s %s: error is not JSON", tt.method, tt.path)
		}
	}
}

func TestRouterRateLimitsByClientIP(t *testing.T) {
	var cfg config.Config
	cfg.Server.RequestTimeout = time.Second
	limit := ratelimit.New(clock.New(), []config.RateWindow{{Limit: 3, Period: time.Hour}})
	h, err := newRouter(&dependencies{Config: cfg, Health: health.New(okPinger{}, time.Second), IPLimit: limit})
	if err != nil {
		t.Fatal(err)
	}
	status := func(path, remote string) int {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.RemoteAddr = remote
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		return rr.Code
	}

	for range 3 {
		status("/api/v1/nope", "203.0.113.7:1000")
	}
	if got := status("/api/v1/nope", "203.0.113.7:2000"); got != http.StatusTooManyRequests {
		t.Fatalf("over the limit got %d, want 429", got)
	}
	if got := status("/livez", "203.0.113.7:1000"); got != http.StatusOK {
		t.Fatalf("health probes must not be limited, got %d", got)
	}
	if got := status("/api/v1/nope", "198.51.100.1:1000"); got != http.StatusNotFound {
		t.Fatalf("another client got %d", got)
	}
	for range 3 {
		status("/api/v1/nope", "[2001:db8:1:2::1]:1000")
	}
	if got := status("/api/v1/nope", "[2001:db8:1:2::ffff]:1000"); got != http.StatusTooManyRequests {
		t.Fatalf("another address in the same /64 got %d, want 429", got)
	}
}

func TestStartupWarnings(t *testing.T) {
	var cfg config.Config
	cfg.App.Environment = config.Production
	cfg.Security.BotCheck = config.BotCheckOff
	if len(startupWarnings(cfg)) != 2 {
		t.Fatal("production without a client IP header and with the bot check off should warn twice")
	}
	cfg.Server.ClientIPHeader = "Fly-Client-IP"
	cfg.Security.BotCheck = config.BotCheckTurnstile
	if len(startupWarnings(cfg)) != 0 {
		t.Fatal("no warning expected once the header is set and the bot check is on")
	}
	cfg.Server.ClientIPHeader = ""
	cfg.App.Environment = config.Development
	if len(startupWarnings(cfg)) != 0 {
		t.Fatal("local development has no proxy, so no warning")
	}
}
