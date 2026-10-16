package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/phone"
	"github.com/coniungo-repo/golang-server/internal/platform/ratelimit"
	"github.com/coniungo-repo/golang-server/internal/platform/sms"
	"github.com/coniungo-repo/golang-server/internal/users"
	"github.com/supertokens/supertokens-golang/ingredients/emaildelivery"
	"github.com/supertokens/supertokens-golang/ingredients/smsdelivery"
)

type recorder struct {
	mu     sync.Mutex
	emails []email.Message
	texts  []sms.Message
	err    error
}

func (r *recorder) Send(_ context.Context, m email.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.emails = append(r.emails, m)
	return r.err
}

type smsRecorder struct{ *recorder }

func (r smsRecorder) Send(_ context.Context, m sms.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.texts = append(r.texts, m)
	return r.err
}

func testService(t *testing.T) (*Service, *recorder) {
	t.Helper()
	phones, err := phone.New(phone.WithDefaultRegion("NG"))
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	return &Service{deps: Deps{
		Config: config.Auth{AppName: "GetGbam", WebsiteDomains: []string{"https://app.example.com", "https://pro.example.com"}},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Phones: phones,
		Email:  rec,
		SMS:    smsRecorder{rec},
	}}, rec
}

func ptr(s string) *string { return &s }

func TestSendEmail(t *testing.T) {
	s, rec := testService(t)
	err := s.sendEmail(emaildelivery.EmailType{PasswordlessLogin: &emaildelivery.PasswordlessLoginType{
		Email: "ada@example.com", UserInputCode: ptr("123456"), CodeLifetime: uint64((10 * time.Minute).Milliseconds()),
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := rec.emails[0]
	if m.To != "ada@example.com" || m.Subject != "123456 is your GetGbam sign-in code" {
		t.Fatalf("unexpected message: %+v", m)
	}
	for _, body := range []string{m.Text, m.HTML} {
		if !strings.Contains(body, "123456") || !strings.Contains(body, "10 minutes") {
			t.Fatalf("body missing code or lifetime: %s", body)
		}
	}
}

func TestSendSMS(t *testing.T) {
	s, rec := testService(t)
	err := s.sendSMS(smsdelivery.SmsType{PasswordlessLogin: &smsdelivery.PasswordlessLoginType{
		PhoneNumber: "+2348031234567", UserInputCode: ptr("654321"), CodeLifetime: 90_000,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "654321 is your GetGbam code. It expires in 2 min. Don't share it with anyone."
	if got := rec.texts[0]; got.To != "+2348031234567" || got.Body != want {
		t.Fatalf("got %+v", got)
	}
	if len(want) > 160 {
		t.Fatal("sms must fit in one segment")
	}
}

func TestSendFailures(t *testing.T) {
	s, rec := testService(t)
	if err := s.sendSMS(smsdelivery.SmsType{PasswordlessLogin: &smsdelivery.PasswordlessLoginType{PhoneNumber: "+2348031234567"}}, nil); !errors.Is(err, errNoCode) {
		t.Fatalf("missing code: got %v", err)
	}
	if err := s.sendEmail(emaildelivery.EmailType{}, nil); err == nil {
		t.Fatal("unsupported email type should fail")
	}
	rec.err = errors.New("provider down")
	err := s.sendEmail(emaildelivery.EmailType{PasswordlessLogin: &emaildelivery.PasswordlessLoginType{Email: "a@b.co", UserInputCode: ptr("1")}}, nil)
	if !errors.Is(err, rec.err) {
		t.Fatalf("provider error should propagate: %v", err)
	}
}

func TestCanonicalise(t *testing.T) {
	s, _ := testService(t)

	e := "  Ada@Example.COM "
	key, err := s.canonicalise(&e, nil)
	if err != nil || e != "ada@example.com" || key != "email:ada@example.com" {
		t.Fatalf("email: %q %q %v", e, key, err)
	}

	p := "0803 123 4567"
	key, err = s.canonicalise(nil, &p)
	if err != nil || p != "+2348031234567" || key != "phone:+2348031234567" {
		t.Fatalf("phone: %q %q %v", p, key, err)
	}

	if _, err := s.canonicalise(nil, nil); !errors.Is(err, errNoContact) {
		t.Fatalf("no contact: %v", err)
	}
}

func TestValidators(t *testing.T) {
	s, _ := testService(t)
	if s.validateEmail("ada@example.com", "public") != nil || s.validatePhone("08031234567", "public") != nil {
		t.Fatal("valid contacts rejected")
	}
	if msg := s.validateEmail("Ada <ada@example.com>", "public"); msg == nil || *msg != invalidEmailMessage {
		t.Fatalf("bad email accepted: %v", msg)
	}
	if msg := s.validatePhone("+44 121 234 5678", "public"); msg == nil || !strings.Contains(*msg, "mobile") {
		t.Fatalf("landline accepted: %v", msg)
	}
	if s.validatePhone(12345, "public") == nil || s.validateEmail(nil, "public") == nil {
		t.Fatal("non-string input accepted")
	}
}

func TestOrigin(t *testing.T) {
	s, _ := testService(t)
	r := httptest.NewRequest("POST", "/auth/signinup/code", nil)
	r.Header.Set("Origin", "https://pro.example.com")
	if o, _ := s.origin(r, nil); o != "https://pro.example.com" {
		t.Fatalf("listed origin not used: %s", o)
	}
	r.Header.Set("Origin", "https://evil.example")
	if o, _ := s.origin(r, nil); o != "https://app.example.com" {
		t.Fatalf("unlisted origin used: %s", o)
	}
}

func TestFingerprintIsKeyedAndNamespaced(t *testing.T) {
	a := &Limiter{secret: []byte("secret-a")}
	b := &Limiter{secret: []byte("secret-b")}
	v := "phone:+2348031234567"
	if string(a.fingerprint("destination", v)) == string(b.fingerprint("destination", v)) {
		t.Fatal("fingerprint must depend on the secret")
	}
	if string(a.fingerprint("destination", v)) == string(a.fingerprint("client_ip", v)) {
		t.Fatal("fingerprint must depend on the kind")
	}
	if strings.Contains(string(a.fingerprint("destination", v)), "803") {
		t.Fatal("fingerprint leaks the raw value")
	}
}

func TestNewRequiresRateLimits(t *testing.T) {
	s, _ := testService(t)
	d := s.deps
	d.Limiter = &Limiter{}
	d.UserLimit = ratelimit.New(clock.New(), nil)
	d.Clock = clock.New()
	d.Users = users.New(nil, d.Clock, "")
	if _, err := New(d); err == nil || !strings.Contains(err.Error(), "rate limits") {
		t.Fatalf("want rate limit error, got %v", err)
	}
}
