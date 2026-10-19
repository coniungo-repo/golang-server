package sms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func termiiServer(t *testing.T, status int, body string, check func(*http.Request, map[string]string)) *Termii {
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
	tm, err := NewTermii(TermiiConfig{APIKey: "termii-key", BaseURL: srv.URL + "/", Sender: "GetGbam", Channel: TermiiChannelDND})
	if err != nil {
		t.Fatal(err)
	}
	return tm
}

var codeText = Message{To: "+2348031234567", Body: "123456 is your GetGbam code."}

func TestTermiiSends(t *testing.T) {
	tm := termiiServer(t, 200, `{"code":"ok","message_id":"9122821270554876574","message":"Successfully Sent","balance":9,"user":"GetGbam"}`, func(r *http.Request, got map[string]string) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/sms/send" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		want := map[string]string{"api_key": "termii-key", "to": "2348031234567", "from": "GetGbam", "sms": codeText.Body, "type": "plain", "channel": "dnd"}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s = %q, want %q", k, got[k], v)
			}
		}
	})
	if err := tm.Send(context.Background(), codeText); err != nil {
		t.Fatal(err)
	}
}

func TestTermiiFailures(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", 401, `{"message":"Invalid api key 'termii-key'"}`},
		{"bad request", 400, `{"message":"Invalid phone 2348031234567"}`},
		{"no message id", 200, `{"message":"queued"}`},
		{"not ok", 200, `{"code":"error","message_id":"1"}`},
		{"not json", 502, `bad gateway`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := termiiServer(t, tt.status, tt.body, nil).Send(context.Background(), codeText)
			if err == nil {
				t.Fatal("want an error")
			}
			for _, leak := range []string{"2348031234567", "termii-key", "123456"} {
				if strings.Contains(err.Error(), leak) {
					t.Fatalf("error leaks %q: %v", leak, err)
				}
			}
		})
	}
}

func TestTermiiRespectsContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	tm, _ := NewTermii(TermiiConfig{APIKey: "k", BaseURL: srv.URL, Sender: "GetGbam", Channel: TermiiChannelDND})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := tm.Send(ctx, codeText); err == nil {
		t.Fatal("want a timeout error")
	}
}

func TestTermiiTimesOutWithoutDeadline(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	tm, _ := NewTermii(TermiiConfig{APIKey: "k", BaseURL: srv.URL, Sender: "GetGbam", Channel: TermiiChannelDND, Timeout: 50 * time.Millisecond})
	if err := tm.Send(context.Background(), codeText); err == nil {
		t.Fatal("want a timeout error")
	}
	if tm, _ := NewTermii(TermiiConfig{APIKey: "k", BaseURL: "https://x.test", Sender: "GetGbam", Channel: TermiiChannelDND}); tm.cfg.Client.Timeout != TermiiDefaultTimeout {
		t.Fatalf("default client timeout %s, want %s", tm.cfg.Client.Timeout, TermiiDefaultTimeout)
	}
}

func TestNewTermiiRequiresSettings(t *testing.T) {
	for name, cfg := range map[string]TermiiConfig{
		"no key":      {BaseURL: "https://x.test", Sender: "GetGbam", Channel: TermiiChannelDND},
		"bad channel": {APIKey: "k", BaseURL: "https://x.test", Sender: "GetGbam", Channel: "whatsapp"},
	} {
		if _, err := NewTermii(cfg); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
