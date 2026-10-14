package request

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type payload struct {
	Name string `json:"name"`
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		wantStatus  int
	}{
		{"ok", `{"name":"a"}`, "application/json", 0},
		{"ok with charset", `{"name":"a"}`, "application/json; charset=utf-8", 0},
		{"wrong media type", `{"name":"a"}`, "text/plain", http.StatusUnsupportedMediaType},
		{"empty", ``, "application/json", http.StatusBadRequest},
		{"syntax", `{"name":`, "application/json", http.StatusBadRequest},
		{"unknown field", `{"nope":1}`, "application/json", http.StatusBadRequest},
		{"wrong type", `{"name":1}`, "application/json", http.StatusBadRequest},
		{"two values", `{"name":"a"}{}`, "application/json", http.StatusBadRequest},
		{"too large", `{"name":"` + strings.Repeat("a", MaxBodyBytes) + `"}`, "application/json", http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			var dst payload
			err := Decode(httptest.NewRecorder(), req, &dst)

			if tt.wantStatus == 0 {
				if err != nil || dst.Name != "a" {
					t.Fatalf("unexpected: %v %+v", err, dst)
				}
				return
			}
			var reqErr *Error
			if !errors.As(err, &reqErr) || reqErr.Status != tt.wantStatus {
				t.Fatalf("want status %d, got %v", tt.wantStatus, err)
			}
		})
	}
}
