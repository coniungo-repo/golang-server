package config

import (
	"slices"
	"strings"
	"testing"
	"time"
)

const validSecret = "0123456789abcdef0123456789abcdef"

func env(overrides map[string]string) func(string) (string, bool) {
	base := map[string]string{
		"DATABASE_URL":         "postgres://u:p@localhost:5432/db?sslmode=disable",
		"FINGERPRINT_SECRET":   validSecret,
		"SUPERTOKENS_APP_NAME": "Test",
	}
	for k, v := range overrides {
		base[k] = v
	}
	return func(k string) (string, bool) {
		v, ok := base[k]
		return v, ok
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := LoadFrom(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App.Environment != Development || cfg.Server.Port != 8080 || cfg.Database.MaxConns != 25 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.Auth.ConnectionURI != "http://localhost:3567" || cfg.Auth.APIDomain != "http://localhost:8080" || cfg.Auth.APIBasePath != "/auth" {
		t.Fatalf("unexpected auth defaults: %+v", cfg.Auth)
	}
	if len(cfg.Auth.WebsiteDomains) == 0 || len(cfg.CORS.AllowedOrigins) != len(cfg.Auth.WebsiteDomains) {
		t.Fatalf("expected matching local website and CORS defaults: %v %v", cfg.Auth.WebsiteDomains, cfg.CORS.AllowedOrigins)
	}
}

func adminEnv(overrides map[string]string) map[string]string {
	base := map[string]string{
		"OIDC_ISSUER":        "https://accounts.google.com",
		"OIDC_CLIENT_ID":     "client",
		"OIDC_CLIENT_SECRET": "secret",
		"OIDC_HOSTED_DOMAIN": "coniungolabs.com",
		"ADMIN_EMAIL_DOMAIN": "coniungolabs.com",
	}
	for k, v := range overrides {
		base[k] = v
	}
	return base
}

func TestAdminConfig(t *testing.T) {
	cfg, err := LoadFrom(env(nil))
	if err != nil || cfg.Admin.Enabled() {
		t.Fatalf("admin sign-in should be off by default: %v", err)
	}
	cfg, err = LoadFrom(env(adminEnv(map[string]string{"ADMIN_EMAIL_DOMAIN": "ConiungoLabs.com", "OIDC_HOSTED_DOMAIN": "coniungolabs.com"})))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Admin.Enabled() || cfg.Admin.EmailDomain != "coniungolabs.com" || cfg.Admin.SessionMaxAge != 12*time.Hour {
		t.Fatalf("unexpected admin config: %+v", cfg.Admin)
	}
	if _, err := LoadFrom(env(adminEnv(map[string]string{"OIDC_ISSUER": "http://127.0.0.1:9999"}))); err != nil {
		t.Fatalf("loopback http issuer should be allowed outside production: %v", err)
	}
}

func prodEnv(overrides map[string]string) map[string]string {
	base := map[string]string{
		"APP_ENV":                     "production",
		"DATABASE_URL":                "postgres://u:p@db.example.com:5432/db?sslmode=verify-full",
		"CORS_ALLOWED_ORIGINS":        "https://app.example.com, https://admin.example.com",
		"SUPERTOKENS_CONNECTION_URI":  "http://supertokens.internal:3567",
		"SUPERTOKENS_API_KEY":         "0123456789abcdefghij0123",
		"SUPERTOKENS_API_DOMAIN":      "https://api.example.com",
		"SUPERTOKENS_WEBSITE_DOMAINS": "https://app.example.com",
		"EMAIL_FROM":                  "GetGbam <hello@getgbam.com>",
		"PHONE_ALLOWED_REGIONS":       "NG",
	}
	for k, v := range overrides {
		base[k] = v
	}
	return base
}

func TestLoadRejects(t *testing.T) {
	prodURL := "postgres://u:p@db.example.com:5432/db?sslmode=verify-full"
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"missing database url", map[string]string{"DATABASE_URL": ""}, "DATABASE_URL: is required"},
		{"bad env", map[string]string{"APP_ENV": "staging"}, "APP_ENV"},
		{"wildcard origin", map[string]string{"CORS_ALLOWED_ORIGINS": "*"}, "CORS_ALLOWED_ORIGINS"},
		{"origin with slash", map[string]string{"CORS_ALLOWED_ORIGINS": "https://app.example.com/"}, "CORS_ALLOWED_ORIGINS"},
		{"min above max", map[string]string{"DB_MIN_CONNS": "30", "DB_MAX_CONNS": "20"}, "DB_MIN_CONNS"},
		{"pool overflow", map[string]string{"DB_MAX_CONNS": "99999999999"}, "DB_MAX_CONNS"},
		{"short secret", map[string]string{"FINGERPRINT_SECRET": "short"}, "FINGERPRINT_SECRET"},
		{"request timeout too long", map[string]string{"HTTP_REQUEST_TIMEOUT": "40s"}, "HTTP_REQUEST_TIMEOUT"},
		{"drain too long", map[string]string{"HTTP_DRAIN_DELAY": "20s"}, "HTTP_DRAIN_DELAY"},
		{"prod sslmode require", map[string]string{"APP_ENV": "production", "DATABASE_URL": strings.Replace(prodURL, "verify-full", "require", 1)}, "verify-full"},
		{"prod keyword dsn without tls", map[string]string{"APP_ENV": "production", "DATABASE_URL": "host=db.example.com user=u dbname=db sslmode=disable"}, "verify-full"},
		{"prod placeholder secret", prodEnv(map[string]string{"FINGERPRINT_SECRET": "change-me-to-32-or-more-random-characters"}), "FINGERPRINT_SECRET: is still the example placeholder"},
		{"bad connection uri", map[string]string{"SUPERTOKENS_CONNECTION_URI": "localhost:3567"}, "SUPERTOKENS_CONNECTION_URI"},
		{"bad second connection uri", map[string]string{"SUPERTOKENS_CONNECTION_URI": "http://a:3567;ftp://b"}, "SUPERTOKENS_CONNECTION_URI"},
		{"api domain with path", map[string]string{"SUPERTOKENS_API_DOMAIN": "http://localhost:8080/api"}, "SUPERTOKENS_API_DOMAIN"},
		{"base path without slash", map[string]string{"SUPERTOKENS_API_BASE_PATH": "auth"}, "SUPERTOKENS_API_BASE_PATH"},
		{"base path trailing slash", map[string]string{"SUPERTOKENS_WEBSITE_BASE_PATH": "/auth/"}, "SUPERTOKENS_WEBSITE_BASE_PATH"},
		{"website not in cors", map[string]string{"SUPERTOKENS_WEBSITE_DOMAINS": "http://localhost:3000"}, "must also be in CORS_ALLOWED_ORIGINS"},
		{"short api key", map[string]string{"SUPERTOKENS_API_KEY": "short"}, "SUPERTOKENS_API_KEY: must be at least"},
		{"prod missing auth", map[string]string{"APP_ENV": "production", "DATABASE_URL": prodURL}, "SUPERTOKENS_CONNECTION_URI: is required"},
		{"prod open phone regions", prodEnv(map[string]string{"PHONE_ALLOWED_REGIONS": ""}), "PHONE_ALLOWED_REGIONS: is required in production"},
		{"prod missing api key", prodEnv(map[string]string{"SUPERTOKENS_API_KEY": ""}), "SUPERTOKENS_API_KEY: is required"},
		{"prod placeholder api key", prodEnv(map[string]string{"SUPERTOKENS_API_KEY": "change-me-to-a-real-api-key"}), "SUPERTOKENS_API_KEY: is still the example placeholder"},
		{"prod http api domain", prodEnv(map[string]string{"SUPERTOKENS_API_DOMAIN": "http://api.example.com"}), "must use https"},
		{"unknown email provider", map[string]string{"EMAIL_PROVIDER": "carrier-pigeon"}, "EMAIL_PROVIDER"},
		{"unknown sms provider", map[string]string{"SMS_PROVIDER": "fax"}, "SMS_PROVIDER"},
		{"bad email from", map[string]string{"EMAIL_FROM": "not an address"}, "EMAIL_FROM"},
		{"prod missing email from", prodEnv(map[string]string{"EMAIL_FROM": ""}), "EMAIL_FROM: is required"},
		{"rate limit garbage", map[string]string{"AUTH_RATE_LIMIT_DESTINATION": "lots"}, "AUTH_RATE_LIMIT_DESTINATION"},
		{"rate limit zero", map[string]string{"AUTH_RATE_LIMIT_DESTINATION": "0/15m"}, "AUTH_RATE_LIMIT_DESTINATION"},
		{"rate limit negative", map[string]string{"AUTH_RATE_LIMIT_CLIENT_IP": "-1/1h"}, "AUTH_RATE_LIMIT_CLIENT_IP"},
		{"rate limit no unit", map[string]string{"AUTH_RATE_LIMIT_CLIENT_IP": "30/60"}, "AUTH_RATE_LIMIT_CLIENT_IP"},
		{"rate limit too long", map[string]string{"AUTH_RATE_LIMIT_CLIENT_IP": "30/720h"}, "AUTH_RATE_LIMIT_CLIENT_IP"},
		{"rate limit repeated period", map[string]string{"AUTH_RATE_LIMIT_DESTINATION": "5/15m,10/15m"}, "repeats the period"},
		{"request limit garbage", map[string]string{"RATE_LIMIT_CLIENT_IP": "fast"}, "RATE_LIMIT_CLIENT_IP"},
		{"request limit zero", map[string]string{"RATE_LIMIT_USER": "0/1m"}, "RATE_LIMIT_USER"},
		{"core timeout too long", map[string]string{"SUPERTOKENS_TIMEOUT": "30s"}, "SUPERTOKENS_TIMEOUT"},
		{"core timeout zero", map[string]string{"SUPERTOKENS_TIMEOUT": "0s"}, "SUPERTOKENS_TIMEOUT"},
		{"header limit too small", map[string]string{"HTTP_MAX_HEADER_BYTES": "100"}, "HTTP_MAX_HEADER_BYTES"},
		{"delivery timeout too long", map[string]string{"DELIVERY_TIMEOUT": "25s"}, "DELIVERY_TIMEOUT"},
		{"ready timeout too long", map[string]string{"DB_READY_TIMEOUT": "1m"}, "DB_READY_TIMEOUT"},
		{"cors max age too long", map[string]string{"CORS_MAX_AGE": "48h"}, "CORS_MAX_AGE"},
		{"postmark without token", map[string]string{"EMAIL_PROVIDER": "postmark", "EMAIL_FROM": "a@b.co"}, "POSTMARK_SERVER_TOKEN: is required"},
		{"postmark without from", map[string]string{"EMAIL_PROVIDER": "postmark", "POSTMARK_SERVER_TOKEN": "t"}, "EMAIL_FROM: is required"},
		{"termii without base url", map[string]string{"SMS_PROVIDER": "termii", "TERMII_API_KEY": "k", "SMS_SENDER": "GetGbam"}, "TERMII_BASE_URL: is required"},
		{"termii http base url", map[string]string{"SMS_PROVIDER": "termii", "TERMII_API_KEY": "k", "TERMII_BASE_URL": "http://api.ng.termii.com", "SMS_SENDER": "GetGbam"}, "TERMII_BASE_URL"},
		{"termii long sender", map[string]string{"SMS_PROVIDER": "termii", "TERMII_API_KEY": "k", "TERMII_BASE_URL": "https://api.ng.termii.com", "SMS_SENDER": "GetGbamServices"}, "SMS_SENDER"},
		{"termii whatsapp channel", map[string]string{"SMS_PROVIDER": "termii", "TERMII_API_KEY": "k", "TERMII_BASE_URL": "https://api.ng.termii.com", "SMS_SENDER": "GetGbam", "TERMII_CHANNEL": "whatsapp"}, "TERMII_CHANNEL"},
		{"prod termii generic channel", prodEnv(map[string]string{"EMAIL_PROVIDER": "postmark", "POSTMARK_SERVER_TOKEN": "t", "SMS_PROVIDER": "termii", "TERMII_API_KEY": "k", "TERMII_BASE_URL": "https://api.ng.termii.com", "SMS_SENDER": "GetGbam", "TERMII_CHANNEL": "generic"}), "TERMII_CHANNEL: must be dnd in production"},
		{"metrics addr garbage", map[string]string{"METRICS_ADDR": "9091"}, "METRICS_ADDR"},
		{"metrics on the api port", map[string]string{"METRICS_ADDR": ":8080"}, "METRICS_ADDR: must use a different port"},
		{"sms total garbage", map[string]string{"AUTH_RATE_LIMIT_SMS_TOTAL": "lots"}, "AUTH_RATE_LIMIT_SMS_TOTAL"},
		{"bot check unknown", map[string]string{"BOT_CHECK": "captcha"}, "BOT_CHECK"},
		{"turnstile without secret", map[string]string{"BOT_CHECK": "turnstile"}, "TURNSTILE_SECRET_KEY: is required"},
		{"bot check timeout too long", map[string]string{"BOT_CHECK": "turnstile", "TURNSTILE_SECRET_KEY": "s", "BOT_CHECK_TIMEOUT": "30s"}, "BOT_CHECK_TIMEOUT"},
		{"prod turnstile test key", prodEnv(map[string]string{"BOT_CHECK": "turnstile", "TURNSTILE_SECRET_KEY": "1x0000000000000000000000000000000AA"}), "TURNSTILE_SECRET_KEY: is one of Cloudflare's test keys"},
		{"rate limit too many windows", map[string]string{"AUTH_RATE_LIMIT_DESTINATION": "1/1m,2/2m,3/3m,4/4m,5/5m"}, "more than 4"},
		{"admin setting without issuer", map[string]string{"ADMIN_EMAIL_DOMAIN": "coniungolabs.com"}, "OIDC_ISSUER is empty"},
		{"admin issuer missing client", map[string]string{"OIDC_ISSUER": "https://accounts.google.com"}, "OIDC_CLIENT_ID: is required"},
		{"admin issuer with path", adminEnv(map[string]string{"OIDC_ISSUER": "https://accounts.google.com/o/oauth2"}), "OIDC_ISSUER"},
		{"admin http issuer remote", adminEnv(map[string]string{"OIDC_ISSUER": "http://accounts.google.com"}), "must use https"},
		{"admin domain with at", adminEnv(map[string]string{"ADMIN_EMAIL_DOMAIN": "@coniungolabs.com"}), "bare domain"},
		{"hosted domain mismatch", adminEnv(map[string]string{"OIDC_HOSTED_DOMAIN": "other.com"}), "OIDC_HOSTED_DOMAIN"},
		{"admin session too long", adminEnv(map[string]string{"ADMIN_SESSION_MAX_AGE": "720h"}), "ADMIN_SESSION_MAX_AGE"},
		{"admin host with scheme", adminEnv(map[string]string{"ADMIN_API_HOST": "https://admin.example.com"}), "ADMIN_API_HOST"},
		{"prod admin without host", prodEnv(adminEnv(nil)), "ADMIN_API_HOST: is required"},
		{"missing app name", map[string]string{"SUPERTOKENS_APP_NAME": ""}, "SUPERTOKENS_APP_NAME: is required"},
		{"prod http website", prodEnv(map[string]string{"SUPERTOKENS_WEBSITE_DOMAINS": "http://app.example.com", "CORS_ALLOWED_ORIGINS": "http://app.example.com"}), "must use https"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFrom(env(tt.env))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestLoadProduction(t *testing.T) {
	_, err := LoadFrom(env(prodEnv(nil)))
	if err == nil {
		t.Fatal("production must reject the log delivery providers")
	}
	lines := strings.Split(err.Error(), "\n")[1:]
	for _, line := range lines {
		if !strings.HasPrefix(line, "EMAIL_PROVIDER: log") && !strings.HasPrefix(line, "SMS_PROVIDER: log") {
			t.Fatalf("unexpected production error: %s", line)
		}
	}
	if len(lines) != 2 {
		t.Fatalf("want exactly the two provider errors, got %q", lines)
	}
}

func TestSecretRedacts(t *testing.T) {
	s := Secret("hunter2")
	if s.String() != "[redacted]" || s.GoString() != "[redacted]" {
		t.Fatal("secret leaked through fmt")
	}
	if b, _ := s.MarshalJSON(); string(b) != `"[redacted]"` {
		t.Fatalf("secret leaked through json: %s", b)
	}
}

func TestRateLimits(t *testing.T) {
	cfg, err := LoadFrom(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Auth.RateLimits; len(got.Destination) != 2 || got.Destination[0] != (RateWindow{5, 15 * time.Minute}) || got.Retention() != 24*time.Hour {
		t.Fatalf("unexpected defaults: %+v", got)
	}

	cfg, err = LoadFrom(env(map[string]string{
		"AUTH_RATE_LIMIT_DESTINATION": " 3/10m , 10/48h ",
		"AUTH_RATE_LIMIT_CLIENT_IP":   "50/1h",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := RateLimits{
		Destination: []RateWindow{{3, 10 * time.Minute}, {10, 48 * time.Hour}},
		ClientIP:    []RateWindow{{50, time.Hour}},
	}
	if !slices.Equal(cfg.Auth.RateLimits.Destination, want.Destination) || !slices.Equal(cfg.Auth.RateLimits.ClientIP, want.ClientIP) {
		t.Fatalf("got %+v, want %+v", cfg.Auth.RateLimits, want)
	}
	if !slices.Equal(cfg.Auth.RateLimits.SMSTotal, []RateWindow{{1000, time.Hour}, {5000, 24 * time.Hour}}) {
		t.Fatalf("unexpected SMS total default: %+v", cfg.Auth.RateLimits.SMSTotal)
	}
	if cfg.Auth.RateLimits.Retention() != 48*time.Hour {
		t.Fatalf("retention should be the longest window, got %s", cfg.Auth.RateLimits.Retention())
	}
}

func TestRequestLimits(t *testing.T) {
	cfg, err := LoadFrom(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Limits.ClientIP, []RateWindow{{600, time.Minute}}) || !slices.Equal(cfg.Limits.User, []RateWindow{{120, time.Minute}, {3000, time.Hour}}) {
		t.Fatalf("unexpected defaults: %+v", cfg.Limits)
	}

	cfg, err = LoadFrom(env(map[string]string{"RATE_LIMIT_CLIENT_IP": "100/10s,1000/1h", "RATE_LIMIT_USER": "30/1m"}))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Limits.ClientIP, []RateWindow{{100, 10 * time.Second}, {1000, time.Hour}}) || !slices.Equal(cfg.Limits.User, []RateWindow{{30, time.Minute}}) {
		t.Fatalf("got %+v", cfg.Limits)
	}
}

func TestTunables(t *testing.T) {
	cfg, err := LoadFrom(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.MaxHeaderBytes != 64<<10 || cfg.Database.ConnectTimeout != 5*time.Second || cfg.Database.HealthCheckPeriod != time.Minute ||
		cfg.Database.ReadyTimeout != 2*time.Second || cfg.CORS.MaxAge != 10*time.Minute || cfg.Delivery.Timeout != 10*time.Second {
		t.Fatalf("unexpected defaults: server %+v db %+v cors %+v delivery %+v", cfg.Server, cfg.Database, cfg.CORS, cfg.Delivery)
	}

	cfg, err = LoadFrom(env(map[string]string{
		"HTTP_MAX_HEADER_BYTES": "32768", "DB_CONNECT_TIMEOUT": "3s", "DB_HEALTH_CHECK_PERIOD": "30s",
		"DB_READY_TIMEOUT": "1s", "CORS_MAX_AGE": "1h", "DELIVERY_TIMEOUT": "8s",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.MaxHeaderBytes != 32768 || cfg.Database.ConnectTimeout != 3*time.Second || cfg.Database.HealthCheckPeriod != 30*time.Second ||
		cfg.Database.ReadyTimeout != time.Second || cfg.CORS.MaxAge != time.Hour || cfg.Delivery.Timeout != 8*time.Second {
		t.Fatalf("overrides not applied: server %+v db %+v cors %+v delivery %+v", cfg.Server, cfg.Database, cfg.CORS, cfg.Delivery)
	}
}

func TestProductionWithRealProviders(t *testing.T) {
	cfg, err := LoadFrom(env(prodEnv(map[string]string{
		"EMAIL_PROVIDER": "postmark", "POSTMARK_SERVER_TOKEN": "server-token",
		"SMS_PROVIDER": "termii", "TERMII_API_KEY": "termii-key", "TERMII_BASE_URL": "https://api.ng.termii.com", "SMS_SENDER": "GetGbam",
	})))
	if err != nil {
		t.Fatal(err)
	}
	d := cfg.Delivery
	if d.PostmarkStream != "outbound" || d.TermiiChannel != "dnd" || d.PostmarkServerToken.Reveal() != "server-token" || d.TermiiAPIKey.Reveal() != "termii-key" {
		t.Fatalf("unexpected delivery config: %+v", d)
	}
}
