package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/logger"
	"github.com/golang-jwt/jwt/v5"
	"github.com/supertokens/supertokens-golang/supertokens"
)

const (
	testClientID = "test-client"
	adminHost    = "admin.test"
)

var codePattern = regexp.MustCompile(`body="(\d{6}) `)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) lastCode(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	m := codePattern.FindAllStringSubmatch(s.b.String(), -1)
	if len(m) == 0 {
		t.Fatal("no code was sent")
	}
	return m[len(m)-1][1]
}

type client struct {
	t    *testing.T
	base string
}

type result struct {
	status int
	body   map[string]any
	token  string
}

func (c client) do(method, path, host, token string, body any) result {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("st-auth-mode", "header")
	if strings.HasPrefix(path, "/auth/signinup") && !strings.Contains(path, "/code") {
		req.Header.Set("rid", "thirdparty")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if host != "" {
		req.Host = host
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	out := result{status: resp.StatusCode, token: resp.Header.Get("st-access-token")}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func (c client) phoneSignIn(logs *syncBuffer, phone string) result {
	c.t.Helper()
	created := c.do("POST", "/auth/signinup/code", "", "", map[string]string{"phoneNumber": phone})
	if created.body["status"] != "OK" {
		c.t.Fatalf("create code: %v", created.body)
	}
	return c.do("POST", "/auth/signinup/code/consume", "", "", map[string]string{
		"deviceId": created.body["deviceId"].(string), "preAuthSessionId": created.body["preAuthSessionId"].(string), "userInputCode": logs.lastCode(c.t),
	})
}

func (c client) adminSignIn(host, code string) result {
	c.t.Helper()
	return c.do("POST", "/auth/signinup", host, "", map[string]any{
		"thirdPartyId": "staff",
		"redirectURIInfo": map[string]any{
			"redirectURIOnProviderDashboard": "http://localhost:5173/auth/callback/staff",
			"redirectURIQueryParams":         map[string]string{"code": code},
			"pkceCodeVerifier":               "verifier",
		},
	})
}

func randomPhone() string {
	return fmt.Sprintf("+234803%07d", rand.IntN(10_000_000))
}

func randomStaff() string {
	return fmt.Sprintf("staff%d@coniungolabs.com", rand.Int64())
}

// TestAuthEndToEnd drives the real router, database and SuperTokens core. It needs TEST_DATABASE_URL (migrated), TEST_SUPERTOKENS_URI and TEST_SUPERTOKENS_API_KEY.
func TestAuthEndToEnd(t *testing.T) {
	dbURL, stURI, stKey := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_SUPERTOKENS_URI"), os.Getenv("TEST_SUPERTOKENS_API_KEY")
	if dbURL == "" || stURI == "" {
		t.Skip("TEST_DATABASE_URL and TEST_SUPERTOKENS_URI not set")
	}
	idp := newMockOIDC(t)
	srv := httptest.NewUnstartedServer(nil)
	base := "http://" + srv.Listener.Addr().String()

	env := map[string]string{
		"APP_ENV": "test", "DATABASE_URL": dbURL, "FINGERPRINT_SECRET": strings.Repeat("s", 32),
		"SUPERTOKENS_CONNECTION_URI": stURI, "SUPERTOKENS_API_KEY": stKey, "SUPERTOKENS_APP_NAME": "Test",
		"SUPERTOKENS_API_DOMAIN": base, "PHONE_DEFAULT_REGION": "NG", "AUTH_RATE_LIMIT_CLIENT_IP": "100000/1h",
		"OIDC_ISSUER": idp.URL, "OIDC_CLIENT_ID": testClientID, "OIDC_CLIENT_SECRET": "secret",
		"OIDC_HOSTED_DOMAIN": "coniungolabs.com", "ADMIN_EMAIL_DOMAIN": "coniungolabs.com", "ADMIN_API_HOST": adminHost,
	}
	cfg, err := config.LoadFrom(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}

	logs := &syncBuffer{}
	log := logger.New(logs, logger.Options{Level: slog.LevelInfo})
	supertokens.ResetForTest()
	t.Cleanup(supertokens.ResetForTest)
	deps, err := newDependencies(logger.Into(context.Background(), log), cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(deps.close)
	router, err := newRouter(deps)
	if err != nil {
		t.Fatal(err)
	}
	srv.Config.Handler = router
	srv.Start()
	t.Cleanup(srv.Close)
	c := client{t: t, base: base}

	phone := randomPhone()
	first := c.phoneSignIn(logs, phone)
	me := c.do("GET", "/api/v1/me", "", first.token, nil)

	t.Run("phone sign-in creates a customer with our user ID", func(t *testing.T) {
		if first.body["status"] != "OK" || me.status != 200 {
			t.Fatalf("sign-in %v, me %d %v", first.body, me.status, me.body)
		}
		if roles := fmt.Sprint(me.body["roles"]); roles != "[customer]" || me.body["sign_in_method"] != "phone_otp" {
			t.Fatalf("unexpected principal: %v", me.body)
		}
		if uid, _ := me.body["user_id"].(string); len(uid) != 36 {
			t.Fatalf("user_id should be our UUID, got %q", uid)
		}
	})

	t.Run("the same phone in another format is the same user", func(t *testing.T) {
		local := "0" + strings.TrimPrefix(phone, "+234")
		again := c.phoneSignIn(logs, local)
		if got := c.do("GET", "/api/v1/me", "", again.token, nil).body["user_id"]; got != me.body["user_id"] {
			t.Fatalf("got user %v, want %v", got, me.body["user_id"])
		}
	})

	t.Run("customers cannot reach admin routes", func(t *testing.T) {
		if got := c.do("GET", "/api/v1/admin/invites", adminHost, first.token, nil).status; got != 403 {
			t.Fatalf("status %d, want 403", got)
		}
	})

	admin := randomStaff()
	adminSub := fmt.Sprint(rand.Int64())

	t.Run("an uninvited staff account is refused", func(t *testing.T) {
		got := c.adminSignIn(adminHost, idp.code(testClientID, adminSub, admin, nil))
		if got.body["status"] != "GENERAL_ERROR" || got.token != "" {
			t.Fatalf("got %v", got.body)
		}
	})

	if _, err := deps.Users.CreateAdminInvite(context.Background(), admin, nil); err != nil {
		t.Fatal(err)
	}

	refusals := map[string]struct {
		host string
		sub  string
		addr string
		with jwt.MapClaims
	}{
		"wrong host":            {"", adminSub, admin, nil},
		"wrong audience":        {adminHost, adminSub, admin, jwt.MapClaims{"aud": "someone-else"}},
		"wrong issuer":          {adminHost, adminSub, admin, jwt.MapClaims{"iss": "https://evil.example"}},
		"wrong hosted domain":   {adminHost, adminSub, admin, jwt.MapClaims{"hd": "gmail.com"}},
		"no hosted domain":      {adminHost, adminSub, admin, jwt.MapClaims{"hd": nil}},
		"unverified email":      {adminHost, adminSub, admin, jwt.MapClaims{"email_verified": false}},
		"email off the domain":  {adminHost, adminSub, "ada@gmail.com", nil},
		"expired token":         {adminHost, adminSub, admin, jwt.MapClaims{"exp": 1}},
		"multi-audience no azp": {adminHost, adminSub, admin, jwt.MapClaims{"aud": []string{testClientID, "other"}}},
	}
	for name, tc := range refusals {
		t.Run("admin refused: "+name, func(t *testing.T) {
			got := c.adminSignIn(tc.host, idp.code(testClientID, tc.sub, tc.addr, tc.with))
			if got.token != "" || got.status == 200 && got.body["status"] == "OK" {
				t.Fatalf("expected refusal, got %d %v", got.status, got.body)
			}
		})
	}

	t.Run("tokens posted directly are refused", func(t *testing.T) {
		got := c.do("POST", "/auth/signinup", adminHost, "", map[string]any{
			"thirdPartyId": "staff",
			"oAuthTokens":  map[string]string{"id_token": idp.sign(t, jwt.MapClaims{"iss": idp.URL, "aud": testClientID, "sub": adminSub, "email": admin, "email_verified": true, "hd": "coniungolabs.com", "exp": 9999999999})},
		})
		if got.token != "" || got.body["status"] == "OK" {
			t.Fatalf("got %v", got.body)
		}
	})

	adminSession := c.adminSignIn(adminHost, idp.code(testClientID, adminSub, admin, nil))
	adminMe := c.do("GET", "/api/v1/me", adminHost, adminSession.token, nil)

	t.Run("an invited admin signs in", func(t *testing.T) {
		if adminSession.body["status"] != "OK" || fmt.Sprint(adminMe.body["roles"]) != "[admin]" || adminMe.body["sign_in_method"] != "oidc" {
			t.Fatalf("sign-in %v, me %v", adminSession.body, adminMe.body)
		}
		if got := c.do("GET", "/api/v1/admin/invites", adminHost, adminSession.token, nil).status; got != 200 {
			t.Fatalf("admin route status %d", got)
		}
	})

	t.Run("admin routes are hidden off the admin host", func(t *testing.T) {
		if got := c.do("GET", "/api/v1/admin/invites", "", adminSession.token, nil).status; got != 404 {
			t.Fatalf("status %d, want 404", got)
		}
	})

	t.Run("a renamed staff email keeps the same account", func(t *testing.T) {
		renamed := strings.Replace(admin, "staff", "renamed", 1)
		again := c.adminSignIn(adminHost, idp.code(testClientID, adminSub, renamed, nil))
		if got := c.do("GET", "/api/v1/me", adminHost, again.token, nil).body["user_id"]; got != adminMe.body["user_id"] {
			t.Fatalf("got %v, want %v", got, adminMe.body["user_id"])
		}
	})

	t.Run("a reissued email with a new subject is refused", func(t *testing.T) {
		got := c.adminSignIn(adminHost, idp.code(testClientID, "another-sub", admin, nil))
		if got.token != "" {
			t.Fatalf("got %v", got.body)
		}
	})

	t.Run("suspending a customer signs them out and blocks sign-in", func(t *testing.T) {
		uid := me.body["user_id"].(string)
		if got := c.do("POST", "/api/v1/admin/users/"+uid+"/suspend", adminHost, adminSession.token, nil).status; got != 204 {
			t.Fatalf("suspend status %d", got)
		}
		if got := c.do("GET", "/api/v1/me", "", first.token, nil).status; got != 401 {
			t.Fatalf("old session status %d, want 401", got)
		}
		if got := c.phoneSignIn(logs, phone); got.body["status"] != "GENERAL_ERROR" {
			t.Fatalf("suspended sign-in: %v", got.body)
		}
		if got := c.do("POST", "/api/v1/admin/users/"+uid+"/reactivate", adminHost, adminSession.token, nil).status; got != 204 {
			t.Fatalf("reactivate status %d", got)
		}
		if got := c.phoneSignIn(logs, phone); got.body["status"] != "OK" {
			t.Fatalf("reactivated sign-in: %v", got.body)
		}
	})

	t.Run("admins cannot suspend themselves", func(t *testing.T) {
		uid := adminMe.body["user_id"].(string)
		if got := c.do("POST", "/api/v1/admin/users/"+uid+"/suspend", adminHost, adminSession.token, nil).status; got != 409 {
			t.Fatalf("status %d, want 409", got)
		}
	})

	t.Run("revoking an admin's invite removes access and signs them out", func(t *testing.T) {
		second, secondSub := randomStaff(), fmt.Sprint(rand.Int64())
		created := c.do("POST", "/api/v1/admin/invites", adminHost, adminSession.token, map[string]string{"email": second})
		if created.status != 201 {
			t.Fatalf("invite status %d %v", created.status, created.body)
		}
		secondSession := c.adminSignIn(adminHost, idp.code(testClientID, secondSub, second, nil))
		if secondSession.body["status"] != "OK" {
			t.Fatalf("second admin sign-in: %v", secondSession.body)
		}
		id := fmt.Sprint(int64(created.body["id"].(float64)))
		if got := c.do("POST", "/api/v1/admin/invites/"+id+"/revoke", adminHost, adminSession.token, nil).status; got != 204 {
			t.Fatalf("revoke status %d", got)
		}
		if got := c.do("GET", "/api/v1/admin/invites", adminHost, secondSession.token, nil).status; got != 403 {
			t.Fatalf("revoked admin session status %d, want 403", got)
		}
		if got := c.adminSignIn(adminHost, idp.code(testClientID, secondSub, second, nil)); got.token != "" {
			t.Fatalf("revoked admin signed in again: %v", got.body)
		}
	})

	t.Run("invites outside the admin domain are rejected", func(t *testing.T) {
		if got := c.do("POST", "/api/v1/admin/invites", adminHost, adminSession.token, map[string]string{"email": "ada@gmail.com"}).status; got != 422 {
			t.Fatalf("status %d, want 422", got)
		}
	})
}
