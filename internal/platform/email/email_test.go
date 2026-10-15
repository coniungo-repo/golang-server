package email

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	valid := map[string]string{
		"Ada@Example.com":         "ada@example.com",
		"  ada.l+otp@mail.co.uk ": "ada.l+otp@mail.co.uk",
		"a@b-c.io":                "a@b-c.io",
	}
	for in, want := range valid {
		got, err := Normalize(in)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	invalid := []string{
		"",
		"ada",
		"ada@",
		"@example.com",
		"ada@localhost",
		"ada@[127.0.0.1]",
		"Ada <ada@example.com>",
		"ada@example..com",
		"ada@-example.com",
		"ada@exa_mple.com",
		"ada@example.com\r\nBcc: x@y.com",
		"ada,bob@example.com",
		strings.Repeat("a", 65) + "@example.com",
		"ada@" + strings.Repeat("a", 250) + ".com",
	}
	for _, in := range invalid {
		if _, err := Normalize(in); !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("Normalize(%q) = %v, want ErrInvalidAddress", in, err)
		}
	}
}
