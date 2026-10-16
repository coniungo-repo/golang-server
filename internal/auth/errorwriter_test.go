package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorWriter(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		requestID   string
		want        string
	}{
		{"adds the request id to sdk errors", 400, "application/json; charset=utf-8", `{"message":"Please provide preAuthSessionId"}`, "req-1", `{"message":"Please provide preAuthSessionId","request_id":"req-1"}`},
		{"adds it to session errors", 401, "application/json; charset=utf-8", `{"message":"unauthorised access"}`, "req-1", `{"message":"unauthorised access","request_id":"req-1"}`},
		{"keeps an existing request id", 400, "application/json", `{"message":"m","request_id":"req-0"}`, "req-1", `{"message":"m","request_id":"req-0"}`},
		{"leaves successes alone", 200, "application/json", `{"status":"OK"}`, "req-1", `{"status":"OK"}`},
		{"leaves non-json errors alone", 404, "text/plain", `404 page not found`, "req-1", `404 page not found`},
		{"leaves non-object bodies alone", 400, "application/json", `"bad"`, "req-1", `"bad"`},
		{"leaves bodies alone without a request id", 400, "application/json", `{"message":"m"}`, "", `{"message":"m"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			ew := &errorWriter{ResponseWriter: rr}
			ew.Header().Set("Content-Type", tt.contentType)
			ew.WriteHeader(tt.status)
			if _, err := ew.Write([]byte(tt.body)); err != nil {
				t.Fatal(err)
			}
			ew.finish(tt.requestID)

			if rr.Code != tt.status {
				t.Fatalf("status %d, want %d", rr.Code, tt.status)
			}
			if got := rr.Body.String(); got != tt.want {
				t.Fatalf("body %s, want %s", got, tt.want)
			}
		})
	}
}

func TestErrorWriterImplicitOK(t *testing.T) {
	rr := httptest.NewRecorder()
	ew := &errorWriter{ResponseWriter: rr}
	_, _ = ew.Write([]byte("ok"))
	ew.Flush()
	ew.finish("req-1")
	if rr.Code != http.StatusOK || rr.Body.String() != "ok" || !rr.Flushed {
		t.Fatalf("got %d %q flushed=%v", rr.Code, rr.Body.String(), rr.Flushed)
	}
}
