package auth

import (
	"testing"

	"github.com/coniungo-repo/golang-server/internal/users"
	"github.com/supertokens/supertokens-golang/supertokens"
)

func TestParseContact(t *testing.T) {
	s, _ := testService(t)
	cases := []struct {
		name    string
		in      contactRequest
		kind    users.Kind
		subject string
		msg     string
	}{
		{"email is normalised", contactRequest{Email: ptr(" Ada@Example.COM ")}, users.KindEmail, "ada@example.com", ""},
		{"phone is E.164", contactRequest{PhoneNumber: ptr("0803 123 4567")}, users.KindPhone, "+2348031234567", ""},
		{"bad email", contactRequest{Email: ptr("Ada <ada@example.com>")}, "", "", invalidEmailMessage},
		{"neither", contactRequest{}, "", "", oneContactMessage},
		{"both", contactRequest{Email: ptr("ada@example.com"), PhoneNumber: ptr("08031234567")}, "", "", oneContactMessage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, subject, msg := s.parseContact(tc.in)
			if kind != tc.kind || subject != tc.subject || msg != tc.msg {
				t.Fatalf("got %q %q %q", kind, subject, msg)
			}
		})
	}
	if _, _, msg := s.parseContact(contactRequest{PhoneNumber: ptr("+44 121 234 5678")}); msg == "" {
		t.Fatal("landline accepted")
	}
}

func TestLinkingMark(t *testing.T) {
	if isLinking(nil) || isLinking(&map[string]interface{}{}) {
		t.Fatal("unmarked contexts must not skip customer sign-in")
	}
	var empty map[string]interface{}
	uc := supertokens.UserContext(&empty)
	markLinking(uc)
	if !isLinking(uc) {
		t.Fatal("marked context should be linking")
	}
}
