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
	if len(cfg.CORS.AllowedOrigins) != 0 {
		t.Fatalf("expected no default origins, got %v", cfg.CORS.AllowedOrigins)
	}
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
		{"prod placeholder secret", map[string]string{"APP_ENV": "production", "DATABASE_URL": prodURL, "FINGERPRINT_SECRET": "change-me-to-32-or-more-random-characters"}, "placeholder"},
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
	_, err := LoadFrom(env(map[string]string{
		"APP_ENV":              "production",
		"DATABASE_URL":         "postgres://u:p@db.example.com:5432/db?sslmode=verify-full",
		"CORS_ALLOWED_ORIGINS": "https://app.example.com, https://admin.example.com",
	}))
	if err != nil {
		t.Fatal(err)
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
