package config

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	tomlString = regexp.MustCompile(`^\s*([A-Z][A-Z0-9_]*)\s*=\s*"([^"]*)"\s*$`)
	coreImage  = regexp.MustCompile(`supertokens-postgresql:[0-9.]+`)
)

// flySecrets are the settings docs/production.md sets with `fly secrets set`, with stand-in values.
var flySecrets = map[string]string{
	"DATABASE_URL":          "postgres://app:pw@db.example.com:5432/app?sslmode=verify-full",
	"FINGERPRINT_SECRET":    strings.Repeat("f", 64),
	"SUPERTOKENS_API_KEY":   strings.Repeat("k", 48),
	"OIDC_CLIENT_ID":        "client-id",
	"OIDC_CLIENT_SECRET":    "client-secret",
	"POSTMARK_SERVER_TOKEN": "server-token",
	"TERMII_API_KEY":        "termii-key",
	"TERMII_BASE_URL":       "https://api.ng.termii.com",
}

func flyEnv(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	env := map[string]string{}
	inEnv := false
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "[") {
			inEnv = line == "[env]"
			continue
		}
		if m := tomlString.FindStringSubmatch(line); inEnv && m != nil {
			env[m[1]] = m[2]
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	return env
}

func TestFlyConfigsPassProductionValidation(t *testing.T) {
	files, _ := filepath.Glob("../../../fly.*.toml")
	if len(files) == 0 {
		t.Fatal("no fly.*.toml found at the repository root")
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), `release_command = "/migrate"`) {
				t.Fatal(`[deploy] must run release_command = "/migrate", or new code can start against an old schema`)
			}
			env := flyEnv(t, path)
			if _, ok := env["APP_ENV"]; ok {
				t.Fatal("APP_ENV comes from the Dockerfile; do not override it in fly.toml")
			}
			env["APP_ENV"] = "production"
			for k, v := range flySecrets {
				if _, ok := env[k]; ok {
					t.Fatalf("%s is a secret and must not be in fly.toml", k)
				}
				env[k] = v
			}
			if _, err := LoadFrom(func(k string) (string, bool) { v, ok := env[k]; return v, ok }); err != nil {
				t.Fatalf("production would refuse to start:\n%v", err)
			}
		})
	}
}

func TestFlyCoreMatchesCompose(t *testing.T) {
	compose, err := os.ReadFile("../../../deploy/docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	want := coreImage.FindString(string(compose))
	files, _ := filepath.Glob("../../../deploy/fly/supertokens.*.toml")
	if want == "" || len(files) == 0 {
		t.Fatal("core image or Fly core configs not found")
	}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := coreImage.FindString(string(b)); got != want {
			t.Errorf("%s runs %q, but local development runs %q", filepath.Base(path), got, want)
		}
	}
}
