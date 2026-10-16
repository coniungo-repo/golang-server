package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coniungo-repo/golang-server/internal/platform/requestid"
	"github.com/supertokens/supertokens-golang/supertokens"
)

func unmarshalErr(t *testing.T, body string, dst any) error {
	t.Helper()
	err := json.Unmarshal([]byte(body), dst)
	if err == nil {
		t.Fatalf("expected %q to fail to decode", body)
	}
	return err
}

func TestOnAPIError(t *testing.T) {
	var asMap map[string]any
	var asStruct struct {
		ClientType string `json:"clientType"`
	}
	coreErr := fmt.Errorf("querier: %w", unmarshalErr(t, "<html>", &asMap))

	tests := []struct {
		name        string
		body        string
		err         error
		wantStatus  int
		wantMessage string
		wantLevel   string
	}{
		{"not json", "not json", unmarshalErr(t, "not json", &asMap), http.StatusBadRequest, "body contains badly-formed JSON (at character 2)", "INFO"},
		{"truncated json", `{"email":`, unmarshalErr(t, `{"email":`, &asMap), http.StatusBadRequest, "body contains badly-formed JSON (at character 9)", "INFO"},
		{"empty body", "", unmarshalErr(t, "", &asMap), http.StatusBadRequest, "body must not be empty", "INFO"},
		{"not an object", "[]", unmarshalErr(t, "[]", &asMap), http.StatusBadRequest, "body must be a JSON object", "INFO"},
		{"wrong field type", `{"clientType":1}`, unmarshalErr(t, `{"clientType":1}`, &asStruct), http.StatusBadRequest, `body contains the wrong type for field "clientType"`, "INFO"},
		{"sdk bad input", `{}`, supertokens.BadInputError{Msg: "Please provide preAuthSessionId"}, http.StatusBadRequest, "Please provide preAuthSessionId", "INFO"},
		{"too large", `{}`, &http.MaxBytesError{Limit: 10}, http.StatusRequestEntityTooLarge, "body must not be larger than 1048576 bytes", "INFO"},
		{"core sent bad json", `{"email":"ada@example.com"}`, coreErr, http.StatusInternalServerError, "Something went wrong on our side. Try again in a moment.", "ERROR"},
		{"server failure", `{}`, errors.New("core unreachable"), http.StatusInternalServerError, "Something went wrong on our side. Try again in a moment.", "ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := testService(t)
			var logs bytes.Buffer
			s.deps.Log = slog.New(slog.NewTextHandler(&logs, nil))

			r := httptest.NewRequest(http.MethodPost, "/auth/signinup/code/consume", strings.NewReader(tt.body))
			r = r.WithContext(requestid.Into(r.Context(), "req-123"))
			w := httptest.NewRecorder()
			s.onAPIError(tt.err, r, w)

			if w.Code != tt.wantStatus {
				t.Fatalf("status %d, want %d", w.Code, tt.wantStatus)
			}
			if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("content type %q", ct)
			}
			var got struct {
				Message   string `json:"message"`
				RequestID string `json:"request_id"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Message != tt.wantMessage || got.RequestID != "req-123" {
				t.Fatalf("body %+v, want message %q", got, tt.wantMessage)
			}
			if !strings.Contains(logs.String(), "level="+tt.wantLevel) {
				t.Fatalf("want a %s log, got %q", tt.wantLevel, logs.String())
			}
		})
	}
}
