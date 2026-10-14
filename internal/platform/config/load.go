package config

import (
	"errors"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"time"
)

func Load() (Config, error) {
	return LoadFrom(os.LookupEnv)
}

func LoadFrom(lookup func(string) (string, bool)) (Config, error) {
	r := &reader{lookup: lookup}

	env := Environment(r.string("APP_ENV", string(Development)))
	r.oneOf("APP_ENV", string(env), string(Development), string(Test), string(Production))
	isLocal := env != Production

	cfg := Config{
		App: App{
			Name:        r.string("APP_NAME", "api-service"),
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
			RequestTimeout:    r.duration("HTTP_REQUEST_TIMEOUT", 20*time.Second),
		},
		Database: Database{
			URL:          r.secret("DATABASE_URL"),
			MaxOpenConns: r.int("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns: r.int("DB_MAX_IDLE_CONNS", 5),
			ConnMaxIdle:  r.duration("DB_CONN_MAX_IDLE", 15*time.Minute),
		},
		Log: Log{
			Level: r.logLevel("LOG_LEVEL", slog.LevelInfo),
		},
		CORS: CORS{
			AllowedOrigins: r.list("CORS_ALLOWED_ORIGINS", localList(isLocal, "*")),
		},
	}

	validate(r, cfg)

	if len(r.errs) > 0 {
		return Config{}, errors.Join(append([]error{errors.New("config validation failed")}, r.errs...)...)
	}

	return cfg, nil
}

func validate(r *reader, c Config) {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		r.fail("PORT", "invalid port number")
	}
	if c.Server.RequestTimeout >= c.Server.WriteTimeout {
		r.fail("HTTP_REQUEST_TIMEOUT", "must be shorter than HTTP_WRITE_TIMEOUT, so the error can still be sent")
	}

	r.require("DATABASE_URL", c.Database.URL.IsSet())
	if c.Database.URL.IsSet() {
		checkDatabaseURL(r, "DATABASE_URL", c.Database.URL.Reveal(), c.App.IsProduction())
	}

	if c.App.IsProduction() {
		if slices.Contains(c.CORS.AllowedOrigins, "*") {
			r.fail("CORS_ALLOWED_ORIGINS", "cannot be '*' in production")
		}

		if c.Database.MaxOpenConns < 10 {
			r.fail("DB_MAX_OPEN_CONNS", "too small for production")
		}
	}
}

func checkDatabaseURL(r *reader, key, dsn string, prod bool) {
	u, err := url.Parse(dsn)
	if err != nil {
		r.fail(key, "invalid connection string format")
		return
	}

	if prod {
		sslmode := u.Query().Get("sslmode")
		if sslmode != "require" && sslmode != "verify-full" && sslmode != "verify-ca" {
			r.fail(key, "production requires sslmode=require, verify-ca, or verify-full")
		}
	}
}
func localDefault(local bool, value string) string {
	if local {
		return value
	}
	return ""
}

func localList(local bool, values ...string) []string {
	if local {
		return values
	}
	return nil
}
