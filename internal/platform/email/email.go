// Package email defines outbound email and the providers that deliver it.
package email

import (
	"context"
	"errors"
	"net/mail"
	"strings"
)

const (
	maxAddressLen = 254
	maxLocalLen   = 64
)

var ErrInvalidAddress = errors.New("email: invalid address")

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Sender delivers a message. Implementations must respect ctx and be safe for concurrent use.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Normalize trims and lowercases addr and rejects anything that is not a bare, routable mailbox.
func Normalize(addr string) (string, error) {
	a := strings.ToLower(strings.TrimSpace(addr))
	if a == "" || len(a) > maxAddressLen {
		return "", ErrInvalidAddress
	}
	parsed, err := mail.ParseAddress(a)
	if err != nil || parsed.Name != "" || parsed.Address != a {
		return "", ErrInvalidAddress
	}
	local, domain, ok := strings.Cut(a, "@")
	if !ok || local == "" || len(local) > maxLocalLen || !validDomain(domain) {
		return "", ErrInvalidAddress
	}
	return a, nil
}

func validDomain(d string) bool {
	if len(d) > 253 || !strings.Contains(d, ".") || strings.HasPrefix(d, "[") {
		return false
	}
	for label := range strings.SplitSeq(d, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return true
}
