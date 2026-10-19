package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func postmarkServer(t *testing.T, status int, body string, check func(*http.Request, map[string]string)) *Postmark {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]string
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("request body: %v", err)
		}
		if check != nil {
			check(r, got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	p, err := NewPostmark(PostmarkConfig{ServerToken: "server-token", From: "GetGbam <hello@getgbam.com>", Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var codeMessage = Message{To: "ada@example.com", Subject: "123456 is your code", Text: "text 123456", HTML: "<p>123456</p>"}

func TestPostmarkSends(t *testing.T) {
	p := postmarkServer(t, 200, `{"ErrorCode":0,"Message":"OK","MessageID":"abc"}`, func(r *http.Request, got map[string]string) {
		if r.Method != http.MethodPost || r.Header.Get("X-Postmark-Server-Token") != "server-token" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected request: %s %v", r.Method, r.Header)
		}
		want := map[string]string{"From": "GetGbam <hello@getgbam.com>", "To": "ada@example.com", "Subject": "123456 is your code", "TextBody": "text 123456", "HtmlBody": "<p>123456</p>", "MessageStream": "outbound"}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s = %q, want %q", k, got[k], v)
			}
		}
	})
	if err := p.Send(context.Background(), codeMessage); err != nil {
		t.Fatal(err)
	}
}

func TestPostmarkFailures(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"rejected", 422, `{"ErrorCode":300,"Message":"Invalid 'To' address: 'ada@example.com'."}`},
		{"unauthorized", 401, `{"ErrorCode":10,"Message":"Bad or missing Server API token."}`},
		{"error code on 200", 200, `{"ErrorCode":406,"Message":"Inactive recipient"}`},
		{"not json", 502, `<html>bad gateway</html>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := postmarkServer(t, tt.status, tt.body, nil).Send(context.Background(), codeMessage)
			if err == nil {
				t.Fatal("want an error")
			}
			for _, leak := range []string{"ada@example.com", "server-token", "123456"} {
				if strings.Contains(err.Error(), leak) {
					t.Fatalf("error leaks %q: %v", leak, err)
				}
			}
		})
	}
}

func TestPostmarkRespectsContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	p, _ := NewPostmark(PostmarkConfig{ServerToken: "t", From: "a@b.co", Endpoint: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := p.Send(ctx, codeMessage); err == nil {
		t.Fatal("want a timeout error")
	}
}

func TestPostmarkTimesOutWithoutDeadline(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	p, _ := NewPostmark(PostmarkConfig{ServerToken: "t", From: "a@b.co", Endpoint: srv.URL, Timeout: 50 * time.Millisecond})
	if err := p.Send(context.Background(), codeMessage); err == nil {
		t.Fatal("want a timeout error")
	}
	if p, _ := NewPostmark(PostmarkConfig{ServerToken: "t", From: "a@b.co"}); p.cfg.Client.Timeout != PostmarkDefaultTimeout {
		t.Fatalf("default client timeout %s, want %s", p.cfg.Client.Timeout, PostmarkDefaultTimeout)
	}
}

func TestNewPostmarkRequiresSettings(t *testing.T) {
	if _, err := NewPostmark(PostmarkConfig{From: "a@b.co"}); err == nil {
		t.Fatal("want an error without a token")
	}
}
