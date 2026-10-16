package respond

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServerError(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want int
	}{
		{"generic", context.Background(), errors.New("boom"), http.StatusInternalServerError},
		{"deadline", context.Background(), fmt.Errorf("query: %w", context.DeadlineExceeded), http.StatusServiceUnavailable},
		{"client gone", canceled, context.Canceled, StatusClientClosedRequest},
		{"canceled internally", context.Background(), context.Canceled, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			ServerError(rr, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(tt.ctx), tt.err)
			if rr.Code != tt.want {
				t.Fatalf("status %d, want %d", rr.Code, tt.want)
			}
		})
	}
}

func TestTooManyRequests(t *testing.T) {
	for wait, want := range map[time.Duration]string{0: "1", 300 * time.Millisecond: "1", 1500 * time.Millisecond: "2", time.Minute: "60"} {
		rr := httptest.NewRecorder()
		TooManyRequests(rr, httptest.NewRequest(http.MethodGet, "/", nil), wait)
		if rr.Code != http.StatusTooManyRequests || rr.Header().Get("Retry-After") != want {
			t.Fatalf("wait %s: status %d, Retry-After %q, want %q", wait, rr.Code, rr.Header().Get("Retry-After"), want)
		}
	}
}
