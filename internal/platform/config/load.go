package config

import (
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	maxPoolConns         = 1000
	minProductionConns   = 10
	minFingerprintLength = 32
	placeholderPrefix    = "change-me"
)

func Load() (Config, error) {
	return LoadFrom(os.LookupEnv)
}

func LoadFrom(lookup func(string) (string, bool)) (Config, error) {
	r := &reader{lookup: lookup}

	env := Environment(r.string("APP_ENV", string(Development)))
	r.oneOf("APP_ENV", string(env), string(Development), string(Test), string(Production))

	cfg := Config{
		App: App{
			Name:        r.string("APP_NAME", "golang-server"),
			Environment: env,
			Version:     r.string("APP_VERSION", "unknown"),
		},
		Server: Server{
			Host:              r.string("HTTP_HOST", "0.0.0.0"),
			Port:              r.int("PORT", 8080),
			ReadHeaderTimeout: r.duration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:       r.duration("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:      r.duration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:       r.duration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout:   r.duration("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second),
			DrainDelay:        r.durationOrZero("HTTP_DRAIN_DELAY", 0),
			RequestTimeout:    r.duration("HTTP_REQUEST_TIMEOUT", 20*time.Second),
			RequestIDHeader:   r.string("HTTP_REQUEST_ID_HEADER", ""),
		},
		Database: Database{
			URL:         r.secret("DATABASE_URL"),
			MaxConns:    r.int32("DB_MAX_CONNS", 25, 1, maxPoolConns),
			MinConns:    r.int32("DB_MIN_CONNS", 2, 0, maxPoolConns),
			ConnMaxIdle: r.duration("DB_CONN_MAX_IDLE", 15*time.Minute),
		},
		Log: Log{
			Level: r.logLevel("LOG_LEVEL", slog.LevelInfo),
		},
		CORS: CORS{
			AllowedOrigins: r.list("CORS_ALLOWED_ORIGINS", nil),
		},
		Security: Security{
			FingerprintSecret: r.secret("FINGERPRINT_SECRET"),
		},
	}

	validate(r, cfg)

	if len(r.errs) > 0 {
		return Config{}, errors.Join(append([]error{errors.New("config validation failed")}, r.errs...)...)
	}
	return cfg, nil
}

func validate(r *reader, c Config) {
	prod := c.App.IsProduction()

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		r.fail("PORT", "invalid port number")
	}
	if c.Server.RequestTimeout >= c.Server.WriteTimeout {
		r.fail("HTTP_REQUEST_TIMEOUT", "must be shorter than HTTP_WRITE_TIMEOUT, so the error can still be sent")
	}
	if c.Server.DrainDelay >= c.Server.ShutdownTimeout {
		r.fail("HTTP_DRAIN_DELAY", "must be shorter than HTTP_SHUTDOWN_TIMEOUT")
	}

	r.require("DATABASE_URL", c.Database.URL.IsSet())
	if c.Database.URL.IsSet() {
		checkDatabaseURL(r, "DATABASE_URL", c.Database.URL.Reveal(), prod)
	}
	if c.Database.MinConns > c.Database.MaxConns {
		r.fail("DB_MIN_CONNS", "must not exceed DB_MAX_CONNS")
	}

	for _, origin := range c.CORS.AllowedOrigins {
		checkOrigin(r, "CORS_ALLOWED_ORIGINS", origin)
	}

	checkFingerprintSecret(r, c.Security.FingerprintSecret, prod)

	if prod {
		if c.Database.MaxConns < minProductionConns {
			r.fail("DB_MAX_CONNS", "too small for production")
		}
	}
}

func checkDatabaseURL(r *reader, key, dsn string, prod bool) {
	pc, err := pgconn.ParseConfig(dsn)
	if err != nil {
		r.fail(key, "invalid connection string")
		return
	}
	if !prod {
		return
	}
	verified := pc.TLSConfig != nil && !pc.TLSConfig.InsecureSkipVerify
	for _, fb := range pc.Fallbacks {
		if fb.TLSConfig == nil || fb.TLSConfig.InsecureSkipVerify {
			verified = false
		}
	}
	if !verified {
		r.fail(key, "production requires sslmode=verify-full")
	}
}

func checkOrigin(r *reader, key, origin string) {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		r.fail(key, "%q must be scheme://host[:port] with no path or trailing slash", origin)
	}
}

func checkFingerprintSecret(r *reader, s Secret, prod bool) {
	const key = "FINGERPRINT_SECRET"
	r.require(key, s.IsSet())
	if !s.IsSet() {
		return
	}
	if len(s.Reveal()) < minFingerprintLength {
		r.fail(key, "must be at least %d characters", minFingerprintLength)
	}
	if prod && strings.HasPrefix(s.Reveal(), placeholderPrefix) {
		r.fail(key, "is still the example placeholder")
	}
}
