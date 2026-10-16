package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/botcheck"
	"github.com/coniungo-repo/golang-server/internal/platform/metrics"
)

type fakeVerifier struct {
	err       error
	gotToken  string
	gotRemote string
}

func (f *fakeVerifier) Verify(_ context.Context, token, remoteIP string) error {
	f.gotToken, f.gotRemote = token, remoteIP
	return f.err
}

func TestCheckHuman(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		want   string
		metric string
	}{
		{"human", nil, "", `bot_checks_total{result="ok"} 1`},
		{"bot", botcheck.ErrFailed, botCheckFailedMessage, `bot_checks_total{result="failed"} 1`},
		{"turnstile down", errors.New("unreachable"), botCheckUnavailableMessage, `bot_checks_total{result="error"} 1`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := testService(t)
			v := &fakeVerifier{err: tt.err}
			m := metrics.New("test")
			s.deps.BotCheck, s.deps.BotCheckTimeout, s.deps.Metrics = v, time.Second, m

			r := httptest.NewRequest(http.MethodPost, "/auth/signinup/code", nil)
			r.RemoteAddr = "203.0.113.7:1000"
			r.Header.Set(BotCheckHeader, "tok")
			if got := s.checkHuman(r); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if v.gotToken != "tok" || v.gotRemote != "203.0.113.7" {
				t.Fatalf("verifier got token %q, ip %q", v.gotToken, v.gotRemote)
			}
			rr := httptest.NewRecorder()
			m.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			if !strings.Contains(rr.Body.String(), tt.metric) {
				t.Fatalf("missing %q", tt.metric)
			}
		})
	}
}
