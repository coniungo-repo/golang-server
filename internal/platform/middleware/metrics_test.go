package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coniungo-repo/golang-server/internal/platform/metrics"
)

func scrapeMetrics(t *testing.T, m *metrics.Metrics) string {
	t.Helper()
	rr := httptest.NewRecorder()
	m.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return rr.Body.String()
}

func TestMetricsUsesRoutePattern(t *testing.T) {
	m := metrics.New("test")
	mux := http.NewServeMux()
	mux.Handle("GET /items/{id}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	h := Chain(mux, Log, Metrics(m), Routes(mux))

	for _, path := range []string{"/items/1", "/items/2", "/nope"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	body := scrapeMetrics(t, m)
	for _, want := range []string{
		`http_requests_total{method="GET",route="GET /items/{id}",status="418"} 2`,
		`http_requests_total{method="GET",route="unmatched",status="404"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "/items/1") {
		t.Fatal("raw paths must never become labels")
	}
}
