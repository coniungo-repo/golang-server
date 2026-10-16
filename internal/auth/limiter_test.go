package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/metrics"
	"github.com/coniungo-repo/golang-server/internal/users"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testRetention = 24 * time.Hour

// testLimiter needs TEST_DATABASE_URL pointing at a migrated database; the test is skipped without it.
func testLimiter(t *testing.T) (*Limiter, *clock.Mock) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	clk := clock.NewMock(time.Now().Add(-48 * time.Hour).Truncate(time.Second))
	secret := t.Name() + time.Now().String()
	return NewLimiter(pool, secret, clk, testRetention), clk
}

func TestLimiterWindows(t *testing.T) {
	l, clk := testLimiter(t)
	ctx := context.Background()
	policy := Policy{Kind: "test", Windows: []config.RateWindow{{Limit: 2, Period: time.Minute}, {Limit: 3, Period: time.Hour}}}
	try := func() bool {
		ok, err := l.Allow(ctx, Attempt{Policy: policy, Value: "phone:+2348031234567"})
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}

	for i := range 2 {
		if !try() {
			t.Fatalf("attempt %d should pass", i+1)
		}
	}
	if try() {
		t.Fatal("third attempt inside a minute should be blocked")
	}
	clk.Add(2 * time.Minute)
	if !try() {
		t.Fatal("attempt after the short window should pass")
	}
	clk.Add(2 * time.Minute)
	if try() {
		t.Fatal("fourth attempt inside an hour should be blocked by the long window")
	}
	clk.Add(time.Hour)
	if !try() {
		t.Fatal("attempt after the long window should pass")
	}
}

func TestLimiterBlockedAttemptRecordsNothing(t *testing.T) {
	l, _ := testLimiter(t)
	ctx := context.Background()
	tight := Policy{Kind: "tight", Windows: []config.RateWindow{{Limit: 1, Period: time.Hour}}}
	loose := Policy{Kind: "loose", Windows: []config.RateWindow{{Limit: 2, Period: time.Hour}}}

	if ok, _ := l.Allow(ctx, Attempt{tight, "a"}, Attempt{loose, "ip"}); !ok {
		t.Fatal("first should pass")
	}
	if ok, _ := l.Allow(ctx, Attempt{tight, "a"}, Attempt{loose, "ip"}); ok {
		t.Fatal("second should be blocked by tight")
	}
	if ok, _ := l.Allow(ctx, Attempt{tight, "b"}, Attempt{loose, "ip"}); !ok {
		t.Fatal("blocked attempt must not have used up the loose budget")
	}
}

func TestLimiterConcurrent(t *testing.T) {
	l, _ := testLimiter(t)
	policy := Policy{Kind: "burst", Windows: []config.RateWindow{{Limit: 5, Period: time.Hour}}}
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for range 40 {
		wg.Go(func() {
			ok, err := l.Allow(context.Background(), Attempt{policy, "phone:+2348000000000"})
			if err != nil {
				t.Error(err)
			}
			if ok {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 5 {
		t.Fatalf("allowed %d of 40 concurrent attempts, want exactly 5", allowed.Load())
	}
}

func TestLimiterPrune(t *testing.T) {
	l, clk := testLimiter(t)
	ctx := context.Background()
	if _, err := l.Allow(ctx, Attempt{Policy{Kind: "prune", Windows: []config.RateWindow{{Limit: 9, Period: time.Hour}}}, "x"}); err != nil {
		t.Fatal(err)
	}
	clk.Add(testRetention + time.Minute)
	n, err := l.Prune(ctx)
	if err != nil || n < 1 {
		t.Fatalf("pruned %d, err %v", n, err)
	}
}

func TestSMSTotalCapsTextsOnly(t *testing.T) {
	l, clk := testLimiter(t)
	wide := []config.RateWindow{{Limit: 100, Period: time.Hour}}
	m := metrics.New("test")
	s := &Service{
		deps:              Deps{Limiter: l, Users: users.New(l.db, clk, ""), Metrics: m},
		destinationPolicy: Policy{Kind: kindDestination, Windows: wide},
		clientPolicy:      Policy{Kind: kindClientIP, Windows: wide},
		smsTotalPolicy:    Policy{Kind: kindSMSTotal, Windows: []config.RateWindow{{Limit: 2, Period: time.Hour}}},
	}
	send := func(ip, destination string) bool {
		r := httptest.NewRequest(http.MethodPost, "/auth/signinup/code", nil)
		r.RemoteAddr = ip + ":1000"
		ok, err := s.allowSend(r, destination)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}

	if !send("203.0.113.1", "phone:+2348031234561") || !send("203.0.113.2", "phone:+2348031234562") {
		t.Fatal("texts under the total should pass")
	}
	if send("203.0.113.3", "phone:+2348031234563") {
		t.Fatal("a third text from a new IP to a new number should hit the SMS total")
	}
	if !send("203.0.113.3", "email:ada@example.com") {
		t.Fatal("emails must not count against the SMS total")
	}

	returning := fmt.Sprintf("+23480%08d", time.Now().UnixNano()%100000000)
	if _, err := s.deps.Users.ResolveCustomer(context.Background(), users.Identity{
		Kind: users.KindPhone, Subject: returning, SupertokensUserID: "st-" + returning + "-" + t.Name(),
	}); err != nil {
		t.Fatal(err)
	}
	if !send("203.0.113.4", "phone:"+returning) {
		t.Fatal("a number that already has an account must not be held back by the SMS total")
	}

	rr := httptest.NewRecorder()
	m.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(rr.Body.String(), "sms_total_counted_total 2") {
		t.Fatalf("want 2 texts counted toward the total (new numbers only), got:\n%s", grepLines(rr.Body.String(), "sms_total"))
	}
	if got := tooManyCodes("phone:+2348031234563"); got != tooManyTextsMessage {
		t.Fatalf("texts should point to email, got %q", got)
	}
}

func grepLines(s, substr string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, substr) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func TestCodeSendIPLimitGroupsIPv6(t *testing.T) {
	l, clk := testLimiter(t)
	wide := []config.RateWindow{{Limit: 100, Period: time.Hour}}
	s := &Service{
		deps:              Deps{Limiter: l, Users: users.New(l.db, clk, "")},
		destinationPolicy: Policy{Kind: kindDestination, Windows: wide},
		clientPolicy:      Policy{Kind: kindClientIP, Windows: []config.RateWindow{{Limit: 2, Period: time.Hour}}},
		smsTotalPolicy:    Policy{Kind: kindSMSTotal, Windows: wide},
	}
	send := func(remote string, n int) bool {
		r := httptest.NewRequest(http.MethodPost, "/auth/signinup/code", nil)
		r.RemoteAddr = remote
		ok, err := s.allowSend(r, fmt.Sprintf("email:user%d@example.com", n))
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !send("[2001:db8:1:2::1]:1000", 1) || !send("[2001:db8:1:2::2]:1000", 2) {
		t.Fatal("the first two sends from a /64 should pass")
	}
	if send("[2001:db8:1:2:ffff::9]:1000", 3) {
		t.Fatal("rotating addresses inside one /64 must not reset the per-IP limit")
	}
	if !send("[2001:db8:1:3::1]:1000", 4) {
		t.Fatal("another /64 has its own limit")
	}
}
