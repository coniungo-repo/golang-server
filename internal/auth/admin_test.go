package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/users"
)

func adminService() *Service {
	return &Service{deps: Deps{Admin: config.Admin{
		Issuer: "https://accounts.google.com", ClientID: "client", EmailDomain: "coniungolabs.com", HostedDomain: "coniungolabs.com",
	}}}
}

func validClaims() map[string]interface{} {
	return map[string]interface{}{
		"iss": "https://accounts.google.com", "aud": "client", "sub": "123",
		"email": "Ada@ConiungoLabs.com", "email_verified": true, "hd": "coniungolabs.com",
	}
}

func TestValidateAdminClaims(t *testing.T) {
	s := adminService()
	if err := s.validateAdminClaims(validClaims()); err != nil {
		t.Fatalf("valid claims rejected: %v", err)
	}
	bareIssuer := validClaims()
	bareIssuer["iss"] = "accounts.google.com"
	if err := s.validateAdminClaims(bareIssuer); err != nil {
		t.Fatalf("Google's bare issuer form rejected: %v", err)
	}

	for name, mutate := range map[string]func(map[string]interface{}){
		"empty":              func(c map[string]interface{}) { clear(c) },
		"issuer":             func(c map[string]interface{}) { c["iss"] = "https://evil.example" },
		"audience":           func(c map[string]interface{}) { c["aud"] = "other" },
		"no subject":         func(c map[string]interface{}) { delete(c, "sub") },
		"unverified":         func(c map[string]interface{}) { c["email_verified"] = false },
		"verified as string": func(c map[string]interface{}) { c["email_verified"] = "false" },
		"domain":             func(c map[string]interface{}) { c["email"] = "ada@gmail.com" },
		"lookalike domain":   func(c map[string]interface{}) { c["email"] = "ada@evilconiungolabs.com" },
		"hosted domain":      func(c map[string]interface{}) { c["hd"] = "gmail.com" },
		"no hosted domain":   func(c map[string]interface{}) { delete(c, "hd") },
	} {
		c := validClaims()
		mutate(c)
		if err := s.validateAdminClaims(c); !errors.Is(err, errAdminClaims) {
			t.Errorf("%s: want errAdminClaims, got %v", name, err)
		}
	}
}

func TestAudienceContains(t *testing.T) {
	cases := []struct {
		aud, azp interface{}
		want     bool
	}{
		{"client", nil, true},
		{"other", nil, false},
		{[]interface{}{"client"}, nil, true},
		{[]interface{}{"client", "other"}, nil, false},
		{[]interface{}{"client", "other"}, "client", true},
		{[]interface{}{"other", "x"}, "client", false},
		{42, nil, false},
	}
	for _, c := range cases {
		if got := audienceContains(c.aud, c.azp, "client"); got != c.want {
			t.Errorf("aud=%v azp=%v: got %v", c.aud, c.azp, got)
		}
	}
}

func TestSessionClaimsRoundTrip(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	payload := sessionClaims(signIn{Principal: users.Principal{UserID: "u1", Roles: []users.Role{users.RoleAdmin}}, Method: MethodOIDC}, now)

	asJSON := map[string]interface{}{}
	for k, v := range payload {
		asJSON[k] = v
	}
	asJSON[claimAuthTime] = float64(now.Unix())
	asJSON[claimRoles] = []interface{}{"admin"}

	p, ok := principalFromClaims(asJSON)
	if !ok || p.UserID != "u1" || p.Method != MethodOIDC || !p.AuthTime.Equal(now) || !p.Has(users.RoleAdmin) {
		t.Fatalf("round trip failed: %+v %v", p, ok)
	}
	if _, ok := principalFromClaims(map[string]interface{}{"sub": "supertokens-id"}); ok {
		t.Fatal("sessions without our claims must be rejected")
	}
}

func TestPermissions(t *testing.T) {
	admin := Principal{Roles: []users.Role{users.RoleAdmin}}
	customer := Principal{Roles: []users.Role{users.RoleCustomer, users.RoleProvider}}
	if !admin.Can(PermManageAdminInvites) || !admin.Can(PermManageUserStatus) || !admin.Can(PermDeleteUsers) {
		t.Fatal("admin should hold admin permissions")
	}
	if customer.Can(PermManageAdminInvites) || customer.Can(PermManageUserStatus) || customer.Can(PermDeleteUsers) {
		t.Fatal("customers and providers must not hold admin permissions")
	}
}

func TestValidUUID(t *testing.T) {
	for in, want := range map[string]bool{
		"3f2b8c1e-9a4d-4c6e-8b2f-1a2b3c4d5e6f": true,
		"3F2B8C1E-9A4D-4C6E-8B2F-1A2B3C4D5E6F": true,
		"3f2b8c1e9a4d4c6e8b2f1a2b3c4d5e6f":     false,
		"3f2b8c1e-9a4d-4c6e-8b2f-1a2b3c4d5e6g": false,
		"' OR 1=1 --------------------------":  false,
		"":                                     false,
	} {
		if validUUID(in) != want {
			t.Errorf("validUUID(%q) != %v", in, want)
		}
	}
}
