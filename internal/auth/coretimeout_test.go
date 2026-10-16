package auth

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/phone"
	"github.com/coniungo-repo/golang-server/internal/platform/ratelimit"
	"github.com/coniungo-repo/golang-server/internal/platform/sms"
	"github.com/coniungo-repo/golang-server/internal/users"
	"github.com/supertokens/supertokens-golang/supertokens"
)

func TestCoreCallsTimeOut(t *testing.T) {
	release := make(chan struct{})
	core := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(core.Close)
	t.Cleanup(func() { close(release) })

	phones, err := phone.New(phone.WithDefaultRegion("NG"))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	window := []config.RateWindow{{Limit: 1, Period: time.Hour}}
	supertokens.ResetForTest()
	t.Cleanup(supertokens.ResetForTest)
	_, err = New(Deps{
		Config: config.Auth{
			ConnectionURI: core.URL, AppName: "Test", APIDomain: "http://localhost:8080", APIBasePath: "/auth",
			WebsiteDomains: []string{"http://localhost:5173"}, WebsiteBasePath: "/auth", CoreTimeout: 200 * time.Millisecond,
			RateLimits: config.RateLimits{Destination: window, ClientIP: window},
		},
		Clock: clock.New(), Log: log, Phones: phones, Email: email.LogSender{Log: log}, SMS: sms.LogSender{Log: log},
		Users:     users.New(nil, clock.New(), ""),
		Limiter:   NewLimiter(nil, "secret", clock.New(), time.Hour),
		UserLimit: ratelimit.New(clock.New(), nil),
	})
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	err = supertokens.DeleteUser("00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("a hung core should fail the call")
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("call took %s; the core timeout was not applied", took)
	}
}

func TestNewRequiresCoreTimeout(t *testing.T) {
	s, _ := testService(t)
	d := s.deps
	window := []config.RateWindow{{Limit: 1, Period: time.Hour}}
	d.Config.RateLimits = config.RateLimits{Destination: window, ClientIP: window}
	d.Limiter = &Limiter{}
	d.UserLimit = ratelimit.New(clock.New(), nil)
	d.Clock = clock.New()
	d.Users = users.New(nil, d.Clock, "")
	if _, err := New(d); err == nil {
		t.Fatal("want an error without a core timeout")
	}
}
