package bootstrap

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/api"
	"github.com/coniungo-repo/golang-server/internal/auth"
	"github.com/coniungo-repo/golang-server/internal/health"
	"github.com/coniungo-repo/golang-server/internal/platform/botcheck"
	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/phone"
	"github.com/coniungo-repo/golang-server/internal/platform/ratelimit"
	"github.com/coniungo-repo/golang-server/internal/platform/route"
	"github.com/coniungo-repo/golang-server/internal/platform/sms"
	"github.com/coniungo-repo/golang-server/internal/users"
	"github.com/supertokens/supertokens-golang/supertokens"
	"gopkg.in/yaml.v3"
)

type routeRecorder map[string]bool

func (r routeRecorder) Handle(pattern string, _ http.Handler) { r[pattern] = true }

type openAPIDoc struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

var specMethods = []string{"get", "put", "post", "delete", "patch"}

// TestOpenAPIMatchesRoutes fails when a route is registered but not in api/openapi.yaml, or described there but not served. SuperTokens routes under /auth are served by its middleware, so they are only checked one way.
func TestOpenAPIMatchesRoutes(t *testing.T) {
	var doc openAPIDoc
	if err := yaml.Unmarshal(api.OpenAPI, &doc); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, item := range doc.Paths {
		for method := range item {
			if slices.Contains(specMethods, method) && !strings.HasPrefix(path, "/auth/") {
				documented[strings.ToUpper(method)+" "+path] = true
			}
		}
	}

	registered := routeRecorder{}
	deps := &dependencies{Auth: testAuthService(t)}
	newProbe(t).Register(registered)
	registerAPI(route.Prefix(registered, apiPrefix), deps)

	for r := range registered {
		if !documented[r] {
			t.Errorf("%s is served but missing from api/openapi.yaml", r)
		}
	}
	for r := range documented {
		if !registered[r] {
			t.Errorf("%s is in api/openapi.yaml but not served", r)
		}
	}
}

func TestOpenAPIRefsResolve(t *testing.T) {
	var root map[string]any
	if err := yaml.Unmarshal(api.OpenAPI, &root); err != nil {
		t.Fatal(err)
	}
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if ref, ok := n["$ref"].(string); ok && !resolves(root, ref) {
				t.Errorf("unresolved $ref %s", ref)
			}
			for _, c := range n {
				walk(c)
			}
		case []any:
			for _, c := range n {
				walk(c)
			}
		}
	}
	walk(root)
}

func TestDocsServed(t *testing.T) {
	var cfg config.Config
	cfg.Server.RequestTimeout = time.Second
	cfg.Docs.Enabled = true
	h, err := newRouter(&dependencies{Config: cfg, Health: newProbe(t)})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"/openapi.yaml":              "openapi: 3.1.0",
		"/docs/":                     `src="init.js"`,
		"/docs/swagger-ui-bundle.js": "SwaggerUIBundle",
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != 200 || !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("%s: status %d", path, rr.Code)
		}
		if csp := rr.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
			t.Fatalf("%s: CSP %q", path, csp)
		}
	}

	cfg.Docs.Enabled = false
	h, err = newRouter(&dependencies{Config: cfg, Health: newProbe(t)})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if rr.Code != 404 {
		t.Fatalf("disabled docs status %d, want 404", rr.Code)
	}
}

func resolves(root map[string]any, ref string) bool {
	path, ok := strings.CutPrefix(ref, "#/")
	if !ok {
		return false
	}
	var cur any = root
	for _, part := range strings.Split(path, "/") {
		m, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		if cur, ok = m[part]; !ok {
			return false
		}
	}
	return true
}

func newProbe(t *testing.T) *health.Probe {
	t.Helper()
	return health.New(okPinger{}, time.Second)
}

func testAuthService(t *testing.T) *auth.Service {
	t.Helper()
	env := map[string]string{
		"APP_ENV": "test", "DATABASE_URL": "postgres://u:p@localhost:5432/db", "FINGERPRINT_SECRET": strings.Repeat("s", 32),
		"SUPERTOKENS_CONNECTION_URI": "http://localhost:3567", "SUPERTOKENS_APP_NAME": "Test", "PHONE_DEFAULT_REGION": "NG",
		"OIDC_ISSUER": "https://accounts.example.com", "OIDC_CLIENT_ID": "id", "OIDC_CLIENT_SECRET": "secret",
		"OIDC_HOSTED_DOMAIN": "example.com", "ADMIN_EMAIL_DOMAIN": "example.com",
	}
	cfg, err := config.LoadFrom(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	phones, err := phone.New(phone.WithDefaultRegion("NG"))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	supertokens.ResetForTest()
	t.Cleanup(supertokens.ResetForTest)
	s, err := auth.New(auth.Deps{
		Config: cfg.Auth, Admin: cfg.Admin, Clock: clock.New(), Log: log, Phones: phones,
		Users:     users.New(nil, clock.New(), cfg.Admin.EmailDomain),
		Limiter:   auth.NewLimiter(nil, "secret", clock.New(), time.Hour),
		UserLimit: ratelimit.New(clock.New(), nil), DeliveryTimeout: time.Second, BotCheck: botcheck.Off{}, BotCheckTimeout: time.Second,
		Email: email.LogSender{Log: log}, SMS: sms.LogSender{Log: log},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCORSAllowsBotCheckHeader(t *testing.T) {
	if !slices.Contains(testAuthService(t).CORSHeaders(), auth.BotCheckHeader) {
		t.Fatal("browsers must be allowed to send the bot check header")
	}
}
