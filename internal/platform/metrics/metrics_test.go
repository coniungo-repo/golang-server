package metrics

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/sms"
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rr := httptest.NewRecorder()
	m.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return rr.Body.String()
}

func expect(t *testing.T, body string, lines ...string) {
	t.Helper()
	for _, l := range lines {
		if !strings.Contains(body, l) {
			t.Errorf("missing %q", l)
		}
	}
}

type fakeEmail struct{ err error }

func (f fakeEmail) Send(context.Context, email.Message) error { return f.err }

type fakeSMS struct{ err error }

func (f fakeSMS) Send(context.Context, sms.Message) error { return f.err }

func TestMetrics(t *testing.T) {
	m := New("v1.2.3")
	m.ObserveRequest(http.MethodGet, "GET /api/v1/me", 200, 30*time.Millisecond)
	m.ObserveRequest("BREW", "unmatched", 405, time.Millisecond)
	m.RateLimited(LimitUser)
	_ = m.Email(fakeEmail{}).Send(context.Background(), email.Message{})
	_ = m.SMS(fakeSMS{err: errors.New("down")}).Send(context.Background(), sms.Message{})

	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer core.Close()
	resp, err := (&http.Client{Transport: m.CoreTransport(http.DefaultTransport)}).Get(core.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	expect(t, scrape(t, m),
		`app_build_info{version="v1.2.3"} 1`,
		`http_requests_total{method="GET",route="GET /api/v1/me",status="200"} 1`,
		`http_requests_total{method="OTHER",route="unmatched",status="405"} 1`,
		`http_request_duration_seconds_count{method="GET",route="GET /api/v1/me"} 1`,
		`rate_limited_total{limit="user"} 1`,
		`code_deliveries_total{channel="email",result="ok"} 1`,
		`code_deliveries_total{channel="sms",result="error"} 1`,
		`code_delivery_duration_seconds_count{channel="sms"} 1`,
		`supertokens_core_requests_total{code="200"} 1`,
		`supertokens_core_request_duration_seconds_count 1`,
		`go_goroutines`,
	)
}

func TestNilMetricsRecordNothing(t *testing.T) {
	var m *Metrics
	m.ObserveRequest(http.MethodGet, "x", 200, time.Second)
	m.RateLimited(LimitUser)
	m.RegisterPool(nil)
	if m.CoreTransport(http.DefaultTransport) != http.DefaultTransport {
		t.Fatal("nil metrics must leave the transport alone")
	}
	s := fakeEmail{}
	if m.Email(s) != email.Sender(s) {
		t.Fatal("nil metrics must leave the sender alone")
	}
}
