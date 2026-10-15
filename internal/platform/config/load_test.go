package config

import (
	"strings"
	"testing"
)

const validSecret = "0123456789abcdef0123456789abcdef"

func env(overrides map[string]string) func(string) (string, bool) {
	base := map[string]string{
		"DATABASE_URL":       "postgres://u:p@localhost:5432/db?sslmode=disable",
		"FINGERPRINT_SECRET": validSecret,
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
		{"prod missing api key", prodEnv(map[string]string{"SUPERTOKENS_API_KEY": ""}), "SUPERTOKENS_API_KEY: is required"},
		{"prod placeholder api key", prodEnv(map[string]string{"SUPERTOKENS_API_KEY": "change-me-to-a-real-api-key"}), "SUPERTOKENS_API_KEY: is still the example placeholder"},
		{"prod http api domain", prodEnv(map[string]string{"SUPERTOKENS_API_DOMAIN": "http://api.example.com"}), "must use https"},
		{"unknown email provider", map[string]string{"EMAIL_PROVIDER": "carrier-pigeon"}, "EMAIL_PROVIDER"},
		{"unknown sms provider", map[string]string{"SMS_PROVIDER": "fax"}, "SMS_PROVIDER"},
		{"bad email from", map[string]string{"EMAIL_FROM": "not an address"}, "EMAIL_FROM"},
		{"prod missing email from", prodEnv(map[string]string{"EMAIL_FROM": ""}), "EMAIL_FROM: is required"},
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
