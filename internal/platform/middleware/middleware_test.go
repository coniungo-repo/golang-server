package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coniungo-repo/golang-server/internal/platform/logger"
	"github.com/coniungo-repo/golang-server/internal/platform/requestid"
	"github.com/coniungo-repo/golang-server/internal/platform/respond"
)

func badRequestHandler(w http.ResponseWriter, r *http.Request) {
	respond.Error(w, r, http.StatusBadRequest, "bad")
}

func errorBody(t *testing.T, rr *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %q: %v", rr.Body.String(), err)
	}
	return body
}

func TestRequestIDGeneratedAndNotSpoofable(t *testing.T) {
	h := RequestID("")(http.HandlerFunc(badRequestHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(requestid.Header, "attacker-chosen")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	id := rr.Header().Get(requestid.Header)
	if !strings.HasPrefix(id, "req_") {
		t.Fatalf("expected generated id, got %q", id)
	}
	if got := errorBody(t, rr)["request_id"]; got != id {
		t.Fatalf("body request_id %q != header %q", got, id)
	}
}

func TestRequestIDTrustedHeader(t *testing.T) {
	h := RequestID("Fly-Request-Id")(http.HandlerFunc(badRequestHandler))
	for in, wantKept := range map[string]bool{"abc-123": true, "bad id\n": false} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Fly-Request-Id", in)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if kept := rr.Header().Get(requestid.Header) == in; kept != wantKept {
			t.Fatalf("header %q kept=%v, want %v", in, kept, wantKept)
		}
	}
}

func corsHandler(t *testing.T) http.Handler {
	t.Helper()
	mw, err := CORS([]string{"https://app.example.com"}, []string{"Content-Type"})
	if err != nil {
		t.Fatal(err)
	}
	return mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
}

func TestCORS(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		headers    map[string]string
		wantStatus int
		wantACAO   string
	}{
		{"allowed origin post", http.MethodPost, map[string]string{"Origin": "https://app.example.com"}, 200, "https://app.example.com"},
		{"foreign origin post", http.MethodPost, map[string]string{"Origin": "https://evil.example"}, 403, ""},
		{"foreign origin get", http.MethodGet, map[string]string{"Origin": "https://evil.example"}, 200, ""},
		{"no origin server call", http.MethodPost, nil, 200, ""},
		{"cross-site fetch without origin", http.MethodPost, map[string]string{"Sec-Fetch-Site": "cross-site"}, 403, ""},
		{"allowed preflight", http.MethodOptions, map[string]string{"Origin": "https://app.example.com", "Access-Control-Request-Method": "POST"}, 204, "https://app.example.com"},
		{"foreign preflight", http.MethodOptions, map[string]string{"Origin": "https://evil.example", "Access-Control-Request-Method": "POST"}, 204, ""},
	}
	h := corsHandler(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "https://api.example.com/x", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Fatalf("status %d, want %d", rr.Code, tt.wantStatus)
			}
			if got := rr.Header().Get("Access-Control-Allow-Origin"); got != tt.wantACAO {
				t.Fatalf("ACAO %q, want %q", got, tt.wantACAO)
			}
		})
	}
}

func TestCORSRejectsInvalidOrigin(t *testing.T) {
	if _, err := CORS([]string{"*"}, nil); err == nil {
		t.Fatal("expected error for wildcard origin")
	}
}

func TestRecover(t *testing.T) {
	h := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusInternalServerError || errorBody(t, rr)["message"] != respond.ServerErrorMessage {
		t.Fatalf("got %d %s", rr.Code, rr.Body)
	}
}

func TestRecoverAbortsStartedResponse(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		panic("boom")
	}))
	defer func() {
		if p := recover(); p != http.ErrAbortHandler {
			t.Fatalf("expected ErrAbortHandler, got %v", p)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestMaxBytes(t *testing.T) {
	h := MaxBytes(4)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		}
	}))
	for name, req := range map[string]*http.Request{
		"content-length": httptest.NewRequest(http.MethodPost, "/", strings.NewReader("too long")),
		"streamed": func() *http.Request {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("too long"))
			r.ContentLength = -1
			return r
		}(),
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("%s: status %d", name, rr.Code)
		}
	}
}

func TestSecureHeaders(t *testing.T) {
	for _, hsts := range []bool{true, false} {
		rr := httptest.NewRecorder()
		SecureHeaders(hsts)(http.NotFoundHandler()).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("missing nosniff")
		}
		if (rr.Header().Get("Strict-Transport-Security") != "") != hsts {
			t.Fatalf("hsts=%v mismatch", hsts)
		}
	}
}

func TestLogUsesRoutePattern(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, logger.Options{Level: slog.LevelInfo})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /things/{id}", func(http.ResponseWriter, *http.Request) {})

	h := Chain(mux, Log, Routes(mux))
	for _, path := range []string{"/things/42", "/nope"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		h.ServeHTTP(httptest.NewRecorder(), req.WithContext(logger.Into(req.Context(), log)))
	}
	out := buf.String()
	if !strings.Contains(out, `route="GET /things/{id}"`) || !strings.Contains(out, "route=unmatched") {
		t.Fatalf("unexpected log output:\n%s", out)
	}
}
