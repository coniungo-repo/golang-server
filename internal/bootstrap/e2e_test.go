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

var codePattern = regexp.MustCompile(`(?:body|subject)="(\d{6}) `)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
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

type rawBody string

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
	switch b := body.(type) {
	case nil:
	case rawBody:
		r = strings.NewReader(string(b))
	default:
		raw, _ := json.Marshal(b)
		r = bytes.NewReader(raw)
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

func (c client) emailSignIn(logs *syncBuffer, addr string) result {
	c.t.Helper()
	created := c.do("POST", "/auth/signinup/code", "", "", map[string]string{"email": addr})
	if created.body["status"] != "OK" {
		c.t.Fatalf("create code: %v", created.body)
	}
	return c.do("POST", "/auth/signinup/code/consume", "", "", map[string]string{
		"deviceId": created.body["deviceId"].(string), "preAuthSessionId": created.body["preAuthSessionId"].(string), "userInputCode": logs.lastCode(c.t),
	})
}

func (c client) startContact(token string, body map[string]string) result {
	c.t.Helper()
	return c.do("POST", "/api/v1/me/identities", "", token, body)
}

func (c client) verifyContact(token string, started result, code string) result {
	c.t.Helper()
	return c.do("POST", "/api/v1/me/identities/verify", "", token, map[string]string{
		"device_id": started.body["device_id"].(string), "pre_auth_session_id": started.body["pre_auth_session_id"].(string), "code": code,
	})
}

func (c client) addContact(logs *syncBuffer, token string, body map[string]string) result {
	c.t.Helper()
	started := c.startContact(token, body)
	if started.status != 201 {
		c.t.Fatalf("start: %d %v", started.status, started.body)
	}
	return c.verifyContact(token, started, logs.lastCode(c.t))
}

func (c client) identities(token string) []any {
	c.t.Helper()
	got := c.do("GET", "/api/v1/me/identities", "", token, nil)
	if got.status != 200 {
		c.t.Fatalf("list identities: %d %v", got.status, got.body)
	}
	return got.body["identities"].([]any)
}

func (c client) userID(token string) string {
	c.t.Helper()
	got := c.do("GET", "/api/v1/me", "", token, nil)
	if got.status != 200 {
		c.t.Fatalf("me: %d %v", got.status, got.body)
	}
	return got.body["user_id"].(string)
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

func randomEmail() string {
	return fmt.Sprintf("user%d@example.com", rand.Int64())
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
		"SUPERTOKENS_API_DOMAIN": base, "PHONE_DEFAULT_REGION": "NG", "AUTH_RATE_LIMIT_CLIENT_IP": "100000/1h", "RATE_LIMIT_CLIENT_IP": "100000/1m",
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

	t.Run("malformed auth requests are client errors with a request id", func(t *testing.T) {
		cases := []struct {
			path, host string
			body       rawBody
			want       string
		}{
			{"/auth/signinup/code/consume", "", "not json", "body contains badly-formed JSON (at character 2)"},
			{"/auth/signinup/code", "", "", "body must not be empty"},
			{"/auth/signinup/code", "", "[]", "body must be a JSON object"},
			{"/auth/signinup/code/consume", "", "{}", "Please provide preAuthSessionId"},
			{"/auth/signinup", adminHost, `{"thirdPartyId":1}`, `body contains the wrong type for field "thirdPartyId"`},
		}
		for _, tc := range cases {
			got := c.do("POST", tc.path, tc.host, "", tc.body)
			if got.status != 400 || got.body["message"] != tc.want || got.body["request_id"] == nil {
				t.Errorf("%s %q: %d %v, want 400 %q with a request_id", tc.path, tc.body, got.status, got.body, tc.want)
			}
		}
		if got := c.do("POST", "/auth/signout", "", "", nil); got.status != 401 || got.body["request_id"] == nil {
			t.Errorf("signout without a session: %d %v, want 401 with a request_id", got.status, got.body)
		}
		if strings.Contains(logs.String(), "auth request failed") {
			t.Fatal("client mistakes were logged as server errors")
		}
	})

	t.Run("the same phone in another format is the same user", func(t *testing.T) {
		local := "0" + strings.TrimPrefix(phone, "+234")
		again := c.phoneSignIn(logs, local)
		if got := c.do("GET", "/api/v1/me", "", again.token, nil).body["user_id"]; got != me.body["user_id"] {
			t.Fatalf("got user %v, want %v", got, me.body["user_id"])
		}
	})

	addr := randomEmail()
	var emailSession result

	t.Run("a customer adds an email and can sign in with it", func(t *testing.T) {
		added := c.addContact(logs, first.token, map[string]string{"email": strings.ToUpper(addr)})
		if added.status != 201 || len(added.body["identities"].([]any)) != 2 {
			t.Fatalf("verify: %d %v", added.status, added.body)
		}
		emailSession = c.emailSignIn(logs, addr)
		if got := c.userID(emailSession.token); got != me.body["user_id"] {
			t.Fatalf("email sign-in got user %v, want %v", got, me.body["user_id"])
		}
	})

	t.Run("a contact already on the account is refused before sending", func(t *testing.T) {
		if got := c.startContact(first.token, map[string]string{"email": addr}); got.status != 409 {
			t.Fatalf("status %d, want 409", got.status)
		}
	})

	t.Run("a wrong code does not link", func(t *testing.T) {
		started := c.startContact(first.token, map[string]string{"email": randomEmail()})
		code := logs.lastCode(t)
		wrong := fmt.Sprintf("%06d", (int(code[0]-'0')+1)%10*100000+int(code[1]-'0')*10000)
		if got := c.verifyContact(first.token, started, wrong); got.status != 422 {
			t.Fatalf("status %d, want 422", got.status)
		}
		if got := len(c.identities(first.token)); got != 2 {
			t.Fatalf("identities %d, want 2", got)
		}
	})

	other := c.phoneSignIn(logs, randomPhone())

	t.Run("a contact on another account is refused without saying whose", func(t *testing.T) {
		started := c.startContact(other.token, map[string]string{"email": addr})
		if started.status != 201 {
			t.Fatalf("start should not reveal the owner: %d %v", started.status, started.body)
		}
		got := c.verifyContact(other.token, started, logs.lastCode(t))
		if got.status != 409 || strings.Contains(fmt.Sprint(got.body), me.body["user_id"].(string)) {
			t.Fatalf("verify: %d %v", got.status, got.body)
		}
		if n := len(c.identities(other.token)); n != 1 {
			t.Fatalf("other account has %d identities, want 1", n)
		}
		if got := c.userID(c.emailSignIn(logs, addr).token); got != me.body["user_id"] {
			t.Fatalf("email moved to %v", got)
		}
	})

	t.Run("the last sign-in method cannot be removed", func(t *testing.T) {
		id := c.identities(other.token)[0].(map[string]any)["id"]
		if got := c.do("DELETE", fmt.Sprintf("/api/v1/me/identities/%v", id), "", other.token, nil).status; got != 409 {
			t.Fatalf("status %d, want 409", got)
		}
	})

	t.Run("removing a contact ends its sessions and frees it", func(t *testing.T) {
		var id any
		for _, raw := range c.identities(first.token) {
			if ident := raw.(map[string]any); ident["kind"] == "email" {
				id = ident["id"]
			}
		}
		if got := c.do("DELETE", fmt.Sprintf("/api/v1/me/identities/%v", id), "", first.token, nil).status; got != 204 {
			t.Fatalf("remove status %d", got)
		}
		if got := c.do("GET", "/api/v1/me", "", emailSession.token, nil).status; got != 401 {
			t.Fatalf("removed method's session status %d, want 401", got)
		}
		if got := c.userID(first.token); got != me.body["user_id"] {
			t.Fatalf("phone session lost: %v", got)
		}
		if got := c.userID(c.emailSignIn(logs, addr).token); got == me.body["user_id"] {
			t.Fatal("a removed email should sign in as a new account")
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

	t.Run("revoking an admin's invite removes access, and a new invite restores it", func(t *testing.T) {
		second, secondSub := randomStaff(), fmt.Sprint(rand.Int64())
		created := c.do("POST", "/api/v1/admin/invites", adminHost, adminSession.token, map[string]string{"email": second})
		if created.status != 201 {
			t.Fatalf("invite status %d %v", created.status, created.body)
		}
		secondSession := c.adminSignIn(adminHost, idp.code(testClientID, secondSub, second, nil))
		if secondSession.body["status"] != "OK" {
			t.Fatalf("second admin sign-in: %v", secondSession.body)
		}
		secondUserID := c.userID(secondSession.token)
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

		if got := c.do("POST", "/api/v1/admin/invites", adminHost, adminSession.token, map[string]string{"email": second}).status; got != 201 {
			t.Fatalf("re-invite status %d", got)
		}
		again := c.adminSignIn(adminHost, idp.code(testClientID, secondSub, second, nil))
		if again.body["status"] != "OK" || again.token == "" {
			t.Fatalf("re-invited admin could not sign in: %v", again.body)
		}
		if got := c.do("GET", "/api/v1/admin/invites", adminHost, again.token, nil).status; got != 200 {
			t.Fatalf("re-invited admin got %d on admin routes, want 200", got)
		}
		if got := c.userID(again.token); got != secondUserID {
			t.Fatalf("re-invited admin is user %s, want their original account %s", got, secondUserID)
		}
	})

	t.Run("staff cannot add contacts", func(t *testing.T) {
		if got := c.do("POST", "/api/v1/me/identities", adminHost, adminSession.token, map[string]string{"email": randomEmail()}).status; got != 403 {
			t.Fatalf("status %d, want 403", got)
		}
	})

	t.Run("customers delete their own account", func(t *testing.T) {
		gone := randomPhone()
		session := c.phoneSignIn(logs, gone)
		uid := c.userID(session.token)
		c.addContact(logs, session.token, map[string]string{"email": randomEmail()})
		if got := c.do("DELETE", "/api/v1/me", "", session.token, nil).status; got != 204 {
			t.Fatalf("delete status %d", got)
		}
		if got := c.do("GET", "/api/v1/me", "", session.token, nil).status; got != 401 {
			t.Fatalf("deleted user's session status %d, want 401", got)
		}
		again := c.phoneSignIn(logs, gone)
		if again.body["createdNewUser"] != true || c.userID(again.token) == uid {
			t.Fatalf("SuperTokens user should be gone: %v", again.body)
		}
	})

	t.Run("admins delete a user and it is audited", func(t *testing.T) {
		session := c.phoneSignIn(logs, randomPhone())
		uid := c.userID(session.token)
		if got := c.do("DELETE", "/api/v1/admin/users/"+uid, "", adminSession.token, nil).status; got != 404 {
			t.Fatalf("off admin host status %d, want 404", got)
		}
		if got := c.do("DELETE", "/api/v1/admin/users/"+uid, adminHost, adminSession.token, nil).status; got != 204 {
			t.Fatalf("delete status %d", got)
		}
		if got := c.do("GET", "/api/v1/me", "", session.token, nil).status; got != 401 {
			t.Fatalf("deleted user's session status %d, want 401", got)
		}
		if got := c.do("DELETE", "/api/v1/admin/users/"+uid, adminHost, adminSession.token, nil).status; got != 404 {
			t.Fatalf("second delete status %d, want 404", got)
		}
		var n int
		err := deps.DB.QueryRow(context.Background(),
			"SELECT count(*) FROM admin_audit_log WHERE action = 'user.deleted' AND target = $1 AND actor_id = $2",
			uid, adminMe.body["user_id"]).Scan(&n)
		if err != nil || n != 1 {
			t.Fatalf("audit rows %d, err %v", n, err)
		}
	})

	t.Run("admins cannot delete themselves", func(t *testing.T) {
		uid := adminMe.body["user_id"].(string)
		if got := c.do("DELETE", "/api/v1/admin/users/"+uid, adminHost, adminSession.token, nil).status; got != 409 {
			t.Fatalf("status %d, want 409", got)
		}
	})

	t.Run("invites outside the admin domain are rejected", func(t *testing.T) {
		if got := c.do("POST", "/api/v1/admin/invites", adminHost, adminSession.token, map[string]string{"email": "ada@gmail.com"}).status; got != 422 {
			t.Fatalf("status %d, want 422", got)
		}
	})

	t.Run("a signed-in user over the per-user limit gets 429", func(t *testing.T) {
		busy := c.phoneSignIn(logs, randomPhone())
		limit := cfg.Limits.User[0].Limit
		for i := range limit {
			if got := c.do("GET", "/api/v1/me", "", busy.token, nil).status; got != 200 {
				t.Fatalf("request %d: status %d within the limit", i+1, got)
			}
		}
		got := c.do("GET", "/api/v1/me", "", busy.token, nil)
		if got.status != 429 || got.body["request_id"] == nil {
			t.Fatalf("over the limit: %d %v, want 429 with a request_id", got.status, got.body)
		}
		if other := c.do("GET", "/api/v1/me", "", first.token, nil).status; other != 200 {
			t.Fatalf("another user from the same IP got %d", other)
		}
	})

	t.Run("metrics reflect the traffic", func(t *testing.T) {
		rr := httptest.NewRecorder()
		deps.Metrics.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		body := rr.Body.String()
		for _, want := range []string{
			`http_requests_total{method="GET",route="GET /api/v1/me",status="200"}`,
			`http_requests_total{method="POST",route="/auth/*",status="200"}`,
			`rate_limited_total{limit="user"}`,
			`code_deliveries_total{channel="sms",result="ok"}`,
			`code_deliveries_total{channel="email",result="ok"}`,
			`supertokens_core_requests_total{code="200"}`,
			`db_pool_connections{state="idle"}`,
			`db_pool_max_connections`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("missing %q", want)
			}
		}
	})
}
