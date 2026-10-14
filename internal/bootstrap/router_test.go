package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/internal/health"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/requestid"
)

type okPinger struct{}

func (okPinger) Ping(context.Context) error { return nil }

func testRouter(t *testing.T) http.Handler {
	t.Helper()
	var cfg config.Config
	cfg.Server.RequestTimeout = time.Second
	h, err := newRouter(&dependencies{Config: cfg, Health: health.New(okPinger{})})
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
