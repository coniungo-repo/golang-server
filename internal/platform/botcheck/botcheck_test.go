package botcheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testSecret = "turnstile-secret"
	testToken  = "token-from-widget"
)

type fakeSiteverify struct {
	mu        sync.Mutex
	responses []string
	statuses  []int
	forms     []map[string]string
}

func (f *fakeSiteverify) server(t *testing.T) *Turnstile {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("form: %v", err)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		form := map[string]string{}
		for k := range r.PostForm {
			form[k] = r.PostForm.Get(k)
		}
		f.forms = append(f.forms, form)
		i := len(f.forms) - 1
		w.WriteHeader(f.statuses[i])
		_, _ = w.Write([]byte(f.responses[i]))
	}))
	t.Cleanup(srv.Close)
	ts, err := NewTurnstile(TurnstileConfig{Secret: testSecret, Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestTurnstileVerify(t *testing.T) {
	tests := []struct {
		name      string
		statuses  []int
		responses []string
		want      error
		wantCalls int
		wantOther bool
	}{
		{"passes", []int{200}, []string{`{"success":true}`}, nil, 1, false},
		{"bad token", []int{200}, []string{`{"success":false,"error-codes":["invalid-input-response"]}`}, ErrFailed, 1, false},
		{"reused token", []int{200}, []string{`{"success":false,"error-codes":["timeout-or-duplicate"]}`}, ErrFailed, 1, false},
		{"retries an internal error", []int{200, 200}, []string{`{"success":false,"error-codes":["internal-error"]}`, `{"success":true}`}, nil, 2, false},
		{"outage fails closed", []int{503, 503}, []string{`oops`, `oops`}, nil, 2, true},
		{"wrong secret fails closed", []int{200}, []string{`{"success":false,"error-codes":["invalid-input-secret"]}`}, nil, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeSiteverify{statuses: tt.statuses, responses: tt.responses}
			err := f.server(t).Verify(context.Background(), testToken, "203.0.113.7")
			switch {
			case tt.wantOther:
				if err == nil || errors.Is(err, ErrFailed) {
					t.Fatalf("want a non-ErrFailed error so callers refuse, got %v", err)
				}
				if strings.Contains(err.Error(), testSecret) || strings.Contains(err.Error(), testToken) {
					t.Fatalf("error leaks the secret or token: %v", err)
				}
			case !errors.Is(err, tt.want) || (tt.want == nil && err != nil):
				t.Fatalf("got %v, want %v", err, tt.want)
			}
			if len(f.forms) != tt.wantCalls {
				t.Fatalf("%d calls, want %d", len(f.forms), tt.wantCalls)
			}
			first := f.forms[0]
			if first["secret"] != testSecret || first["response"] != testToken || first["remoteip"] != "203.0.113.7" || len(first["idempotency_key"]) != 36 {
				t.Fatalf("unexpected form: %v", first)
			}
			for _, form := range f.forms[1:] {
				if form["idempotency_key"] != first["idempotency_key"] {
					t.Fatal("a retry must reuse the idempotency key")
				}
			}
		})
	}
}

func TestTurnstileRejectsMissingTokenWithoutCalling(t *testing.T) {
	f := &fakeSiteverify{}
	ts := f.server(t)
	for _, token := range []string{"", strings.Repeat("x", maxTokenLen+1)} {
		if err := ts.Verify(context.Background(), token, ""); !errors.Is(err, ErrFailed) {
			t.Fatalf("got %v, want ErrFailed", err)
		}
	}
	if len(f.forms) != 0 {
		t.Fatal("no request should be made without a usable token")
	}
}

func TestTurnstileRespectsContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	ts, _ := NewTurnstile(TurnstileConfig{Secret: testSecret, Endpoint: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := ts.Verify(ctx, testToken, ""); err == nil || errors.Is(err, ErrFailed) {
		t.Fatalf("a timeout must fail closed with a non-ErrFailed error, got %v", err)
	}
}

func TestOffAcceptsEverything(t *testing.T) {
	if err := (Off{}).Verify(context.Background(), "", ""); err != nil {
		t.Fatal(err)
	}
}
