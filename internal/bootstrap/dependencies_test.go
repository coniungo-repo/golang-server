package bootstrap

import (
	"io"
	"log/slog"
	"testing"

	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/sms"
)

func TestNewPhoneValidator(t *testing.T) {
	v, err := newPhoneValidator(config.Phone{DefaultRegion: "NG", AllowedRegions: []string{"NG", "GH"}})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Valid("0803 123 4567") {
		t.Fatal("expected local Nigerian mobile to be valid")
	}
	if _, err := newPhoneValidator(config.Phone{DefaultRegion: "XX"}); err == nil {
		t.Fatal("expected unsupported region to fail startup")
	}
}

func TestNewSenders(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := config.Delivery{
		EmailProvider: config.ProviderPostmark, EmailFrom: "GetGbam <hello@getgbam.com>", PostmarkServerToken: config.Secret("t"), PostmarkStream: "outbound",
		SMSProvider: config.ProviderTermii, SMSSender: "GetGbam", TermiiAPIKey: config.Secret("k"), TermiiBaseURL: "https://api.ng.termii.com", TermiiChannel: "dnd",
	}
	if e, err := newEmailSender(d, log); err != nil {
		t.Fatal(err)
	} else if _, ok := e.(*email.Postmark); !ok {
		t.Fatalf("got %T, want Postmark", e)
	}
	if s, err := newSMSSender(d, log); err != nil {
		t.Fatal(err)
	} else if _, ok := s.(*sms.Termii); !ok {
		t.Fatalf("got %T, want Termii", s)
	}
}
