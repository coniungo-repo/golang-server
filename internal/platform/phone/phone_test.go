package phone

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func mustNew(t *testing.T, opts ...Option) *Validator {
	t.Helper()
	v, err := New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestParseValid(t *testing.T) {
	ng := mustNew(t, WithDefaultRegion("NG"))
	tests := []struct {
		in, e164, region string
		kind             Type
	}{
		{"+2348031234567", "+2348031234567", "NG", Mobile},
		{"+234 803 123 4567", "+2348031234567", "NG", Mobile},
		{"08031234567", "+2348031234567", "NG", Mobile},
		{"(0803) 123-4567", "+2348031234567", "NG", Mobile},
		{"  0803.123.4567  ", "+2348031234567", "NG", Mobile},
		{"+44 7400 123456", "+447400123456", "GB", Mobile},
		{"+1 (201) 555-0123", "+12015550123", "US", FixedLineOrMobile},
		{"＋２３４８０３１２３４５６７", "+2348031234567", "NG", Mobile},
		{"+234 803 123 4567", "+2348031234567", "NG", Mobile},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			n, err := ng.Parse(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if n.E164() != tt.e164 || n.Region() != tt.region || n.Type() != tt.kind {
				t.Fatalf("got %s %s %s, want %s %s %s", n.E164(), n.Region(), n.Type(), tt.e164, tt.region, tt.kind)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	ng := mustNew(t, WithDefaultRegion("NG"))
	noRegion := mustNew(t)
	tests := []struct {
		name string
		v    *Validator
		in   string
		want error
	}{
		{"empty", ng, "", ErrEmpty},
		{"whitespace", ng, "   ", ErrEmpty},
		{"too long input", ng, "+" + strings.Repeat("1", maxInputLen), ErrTooLong},
		{"letters", ng, "+1 800 FLOWERS", ErrInvalidCharacters},
		{"extension", ng, "+447400123456 ext 12", ErrInvalidCharacters},
		{"semicolon extension", ng, "+447400123456;ext=12", ErrInvalidCharacters},
		{"double plus", ng, "++2348031234567", ErrInvalidCharacters},
		{"plus in middle", ng, "234+8031234567", ErrInvalidCharacters},
		{"null byte", ng, "+234803\x001234567", ErrInvalidCharacters},
		{"newline", ng, "+234803\n1234567", ErrInvalidCharacters},
		{"invalid utf8", ng, "+234\xff8031234567", ErrInvalidCharacters},
		{"emoji", ng, "+234📱8031234567", ErrInvalidCharacters},
		{"no country code", noRegion, "08031234567", ErrMissingCountryCode},
		{"unknown country code", ng, "+999 123 456 789", ErrInvalidCountryCode},
		{"too short", ng, "+234803", ErrTooShort},
		{"too many digits", ng, "+44 7400 1234567890", ErrTooManyDigits},
		{"not a real number", ng, "+2340001234567", ErrInvalid},
		{"fixed line", ng, "+441212345678", ErrTypeNotAllowed},
		{"toll free", ng, "+18002345678", ErrTypeNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.v.Parse(tt.in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Parse(%q) = %v, want %v", tt.in, err, tt.want)
			}
			if Message(err) == "" {
				t.Fatal("missing user message")
			}
		})
	}
}

func TestAllowedRegions(t *testing.T) {
	v := mustNew(t, WithDefaultRegion("ng"), WithAllowedRegions("ng", "GH"))
	if _, err := v.Parse("08031234567"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Parse("+447400123456"); !errors.Is(err, ErrRegionNotAllowed) || !strings.Contains(Message(err), "email") {
		t.Fatalf("want ErrRegionNotAllowed pointing to email, got %v (%q)", err, Message(err))
	}
}

func TestTypes(t *testing.T) {
	any := mustNew(t, AnyType())
	if n, err := any.Parse("+441212345678"); err != nil || n.Type() != FixedLine {
		t.Fatalf("AnyType should accept fixed line: %v %v", n.Type(), err)
	}
	fixed := mustNew(t, WithTypes(FixedLine))
	if _, err := fixed.Parse("+447400123456"); !errors.Is(err, ErrTypeNotAllowed) {
		t.Fatalf("want ErrTypeNotAllowed, got %v", err)
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	for name, opts := range map[string][]Option{
		"unknown default region":    {WithDefaultRegion("XX")},
		"empty default region":      {WithDefaultRegion("")},
		"unknown allowed region":    {WithAllowedRegions("NG", "ZZ")},
		"default outside allowlist": {WithDefaultRegion("NG"), WithAllowedRegions("GB")},
		"no types":                  {WithTypes()},
	} {
		if _, err := New(opts...); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestNumberIsMasked(t *testing.T) {
	n, err := mustNew(t).Parse("+2348031234567")
	if err != nil {
		t.Fatal(err)
	}
	const masked = "+234********67"
	if n.String() != masked || fmt.Sprintf("%v", n) != masked {
		t.Fatalf("fmt leaked: %s", n)
	}
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "phone", n)
	if strings.Contains(buf.String(), "8031234567") || !strings.Contains(buf.String(), masked) {
		t.Fatalf("slog leaked: %s", buf.String())
	}
}

func TestMessageFallback(t *testing.T) {
	if Message(errors.New("other")) != messages[ErrInvalid] {
		t.Fatal("unknown errors should fall back to the generic message")
	}
	if Message(fmt.Errorf("wrapped: %w", ErrTooShort)) != messages[ErrTooShort] {
		t.Fatal("wrapped errors should map to their message")
	}
}

func TestConcurrentParse(t *testing.T) {
	v := mustNew(t, WithDefaultRegion("NG"))
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 50 {
				if _, err := v.Parse("0803 123 4567"); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{"+2348031234567", "08031234567", "+1 (201) 555-0123", "++1", "+999", "", "０８０３"} {
		f.Add(seed)
	}
	v, err := New(WithDefaultRegion("NG"), AnyType())
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, in string) {
		n, err := v.Parse(in)
		if err != nil {
			return
		}
		again, err := v.Parse(n.E164())
		if err != nil || again != n {
			t.Fatalf("canonical form %q did not round-trip: %v", n.E164(), err)
		}
	})
}
