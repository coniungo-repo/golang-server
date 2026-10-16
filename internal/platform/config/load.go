package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	maxPoolConns         = 1000
	minProductionConns   = 10
	minFingerprintLength = 32
	minAuthAPIKeyLength  = 20
	minHeaderBytes       = 4 << 10
	maxHeaderBytes       = 1 << 20
	maxCORSMaxAge        = 24 * time.Hour
	placeholderPrefix    = "change-me"
)

var (
	defaultDestinationLimits = []RateWindow{{Limit: 5, Period: 15 * time.Minute}, {Limit: 15, Period: 24 * time.Hour}}
	defaultClientIPLimits    = []RateWindow{{Limit: 30, Period: time.Hour}, {Limit: 100, Period: 24 * time.Hour}}
	defaultRequestIPLimits   = []RateWindow{{Limit: 600, Period: time.Minute}}
	defaultRequestUserLimits = []RateWindow{{Limit: 120, Period: time.Minute}, {Limit: 3000, Period: time.Hour}}
)

var localWebsiteDomains = []string{
	"http://localhost:4321",
	"http://localhost:5173",
	"http://localhost:5174",
	"http://localhost:5175",
}

func Load() (Config, error) {
	return LoadFrom(os.LookupEnv)
}

func LoadFrom(lookup func(string) (string, bool)) (Config, error) {
	r := &reader{lookup: lookup}

	env := Environment(r.string("APP_ENV", string(Development)))
	r.oneOf("APP_ENV", string(env), string(Development), string(Test), string(Production))
	local := env != Production
	port := r.int("PORT", 8080)

	cfg := Config{
		App: App{
			Name:        r.string("APP_NAME", "golang-server"),
			Environment: env,
			Version:     r.string("APP_VERSION", "unknown"),
		},
		Server: Server{
			Host:              r.string("HTTP_HOST", "0.0.0.0"),
			Port:              port,
			ReadHeaderTimeout: r.duration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:       r.duration("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:      r.duration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:       r.duration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout:   r.duration("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second),
			DrainDelay:        r.durationOrZero("HTTP_DRAIN_DELAY", 0),
			RequestTimeout:    r.duration("HTTP_REQUEST_TIMEOUT", 20*time.Second),
			RequestIDHeader:   r.string("HTTP_REQUEST_ID_HEADER", ""),
			ClientIPHeader:    r.string("HTTP_CLIENT_IP_HEADER", ""),
			MaxHeaderBytes:    r.int("HTTP_MAX_HEADER_BYTES", 64<<10),
			MetricsAddr:       r.string("METRICS_ADDR", ""),
		},
		Database: Database{
			URL:               r.secret("DATABASE_URL"),
			MaxConns:          r.int32("DB_MAX_CONNS", 25, 1, maxPoolConns),
			MinConns:          r.int32("DB_MIN_CONNS", 2, 0, maxPoolConns),
			ConnMaxIdle:       r.duration("DB_CONN_MAX_IDLE", 15*time.Minute),
			ConnectTimeout:    r.duration("DB_CONNECT_TIMEOUT", 5*time.Second),
			HealthCheckPeriod: r.duration("DB_HEALTH_CHECK_PERIOD", time.Minute),
			ReadyTimeout:      r.duration("DB_READY_TIMEOUT", 2*time.Second),
		},
		Log: Log{
			Level: r.logLevel("LOG_LEVEL", slog.LevelInfo),
		},
		CORS: CORS{
			AllowedOrigins: r.list("CORS_ALLOWED_ORIGINS", localList(local, localWebsiteDomains...)),
			MaxAge:         r.duration("CORS_MAX_AGE", 10*time.Minute),
		},
		Auth: Auth{
			ConnectionURI:   r.string("SUPERTOKENS_CONNECTION_URI", localDefault(local, "http://localhost:3567")),
			APIKey:          r.secret("SUPERTOKENS_API_KEY"),
			AppName:         r.string("SUPERTOKENS_APP_NAME", ""),
			APIDomain:       r.string("SUPERTOKENS_API_DOMAIN", localDefault(local, fmt.Sprintf("http://localhost:%d", port))),
			APIBasePath:     r.path("SUPERTOKENS_API_BASE_PATH", "/auth"),
			WebsiteDomains:  r.list("SUPERTOKENS_WEBSITE_DOMAINS", localList(local, localWebsiteDomains...)),
			WebsiteBasePath: r.path("SUPERTOKENS_WEBSITE_BASE_PATH", "/auth"),
			CoreTimeout:     r.duration("SUPERTOKENS_TIMEOUT", 5*time.Second),
			RateLimits: RateLimits{
				Destination: r.rateWindows("AUTH_RATE_LIMIT_DESTINATION", defaultDestinationLimits),
				ClientIP:    r.rateWindows("AUTH_RATE_LIMIT_CLIENT_IP", defaultClientIPLimits),
			},
		},
		Admin: Admin{
			Issuer:        r.string("OIDC_ISSUER", ""),
			ClientID:      r.string("OIDC_CLIENT_ID", ""),
			ClientSecret:  r.secret("OIDC_CLIENT_SECRET"),
			HostedDomain:  r.string("OIDC_HOSTED_DOMAIN", ""),
			EmailDomain:   strings.ToLower(r.string("ADMIN_EMAIL_DOMAIN", "")),
			APIHost:       strings.ToLower(r.string("ADMIN_API_HOST", "")),
			SessionMaxAge: r.duration("ADMIN_SESSION_MAX_AGE", 12*time.Hour),
		},
		Delivery: Delivery{
			EmailProvider: r.string("EMAIL_PROVIDER", ProviderLog),
			EmailFrom:     r.string("EMAIL_FROM", ""),
			SMSProvider:   r.string("SMS_PROVIDER", ProviderLog),
			SMSSender:     r.string("SMS_SENDER", ""),
			Timeout:       r.duration("DELIVERY_TIMEOUT", 10*time.Second),

			PostmarkServerToken: r.secret("POSTMARK_SERVER_TOKEN"),
			PostmarkStream:      r.string("POSTMARK_MESSAGE_STREAM", "outbound"),

			TermiiAPIKey:  r.secret("TERMII_API_KEY"),
			TermiiBaseURL: r.string("TERMII_BASE_URL", ""),
			TermiiChannel: r.string("TERMII_CHANNEL", "dnd"),
		},
		Security: Security{
			FingerprintSecret: r.secret("FINGERPRINT_SECRET"),
		},
		Docs: Docs{
			Enabled: r.bool("API_DOCS_ENABLED", local),
		},
		Limits: Limits{
			ClientIP: r.rateWindows("RATE_LIMIT_CLIENT_IP", defaultRequestIPLimits),
			User:     r.rateWindows("RATE_LIMIT_USER", defaultRequestUserLimits),
		},
		Phone: Phone{
			DefaultRegion:  r.string("PHONE_DEFAULT_REGION", ""),
			AllowedRegions: r.list("PHONE_ALLOWED_REGIONS", nil),
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
	if c.Server.MaxHeaderBytes < minHeaderBytes || c.Server.MaxHeaderBytes > maxHeaderBytes {
		r.fail("HTTP_MAX_HEADER_BYTES", "must be between %d and %d", minHeaderBytes, maxHeaderBytes)
	}
	if c.Server.MetricsAddr != "" {
		_, port, err := net.SplitHostPort(c.Server.MetricsAddr)
		if err != nil || port == "" {
			r.fail("METRICS_ADDR", "%q must be host:port or :port, e.g. :9091", c.Server.MetricsAddr)
		} else if port == strconv.Itoa(c.Server.Port) {
			r.fail("METRICS_ADDR", "must use a different port from PORT, so metrics are never public")
		}
	}
	if c.Delivery.Timeout >= c.Server.RequestTimeout {
		r.fail("DELIVERY_TIMEOUT", "must be shorter than HTTP_REQUEST_TIMEOUT, since codes are sent during the request")
	}
	if c.Database.ReadyTimeout >= c.Server.RequestTimeout {
		r.fail("DB_READY_TIMEOUT", "must be shorter than HTTP_REQUEST_TIMEOUT")
	}
	if c.CORS.MaxAge > maxCORSMaxAge {
		r.fail("CORS_MAX_AGE", "must be at most %s; browsers cap it lower anyway", maxCORSMaxAge)
	}
	if c.Auth.CoreTimeout >= c.Server.RequestTimeout {
		r.fail("SUPERTOKENS_TIMEOUT", "must be shorter than HTTP_REQUEST_TIMEOUT, so a slow core still gets an error response")
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

	checkAuth(r, c.Auth, c.CORS.AllowedOrigins, prod)
	checkFingerprintSecret(r, c.Security.FingerprintSecret, prod)
	checkDelivery(r, c.Delivery, prod)
	checkAdmin(r, c.Admin, prod)

	if prod {
		if len(c.Phone.AllowedRegions) == 0 {
			r.fail("PHONE_ALLOWED_REGIONS", "is required in production: list the countries you send codes to, or anyone can run up SMS costs on expensive international routes")
		}
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

func checkOrigin(r *reader, key, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		r.fail(key, "%q must be scheme://host[:port] with no path or trailing slash", origin)
		return false
	}
	return true
}

func checkHTTPS(r *reader, key, origin string) {
	if !strings.HasPrefix(origin, "https://") {
		r.fail(key, "%q must use https in production", origin)
	}
}

func checkAuth(r *reader, a Auth, corsOrigins []string, prod bool) {
	r.require("SUPERTOKENS_APP_NAME", a.AppName != "")
	r.require("SUPERTOKENS_CONNECTION_URI", a.ConnectionURI != "")
	if a.ConnectionURI != "" {
		for uri := range strings.SplitSeq(a.ConnectionURI, ";") {
			u, err := url.Parse(uri)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				r.fail("SUPERTOKENS_CONNECTION_URI", "%q is not a valid http(s) URL", uri)
			}
		}
	}

	r.require("SUPERTOKENS_API_DOMAIN", a.APIDomain != "")
	if a.APIDomain != "" && checkOrigin(r, "SUPERTOKENS_API_DOMAIN", a.APIDomain) && prod {
		checkHTTPS(r, "SUPERTOKENS_API_DOMAIN", a.APIDomain)
	}

	r.require("SUPERTOKENS_WEBSITE_DOMAINS", len(a.WebsiteDomains) > 0)
	for _, d := range a.WebsiteDomains {
		if !checkOrigin(r, "SUPERTOKENS_WEBSITE_DOMAINS", d) {
			continue
		}
		if prod {
			checkHTTPS(r, "SUPERTOKENS_WEBSITE_DOMAINS", d)
		}
		if !slices.Contains(corsOrigins, d) {
			r.fail("SUPERTOKENS_WEBSITE_DOMAINS", "%q must also be in CORS_ALLOWED_ORIGINS, or the browser can't call the auth API", d)
		}
	}

	if a.APIKey.IsSet() && len(a.APIKey.Reveal()) < minAuthAPIKeyLength {
		r.fail("SUPERTOKENS_API_KEY", "must be at least %d characters", minAuthAPIKeyLength)
	}
	if prod {
		r.require("SUPERTOKENS_API_KEY", a.APIKey.IsSet())
		if strings.HasPrefix(a.APIKey.Reveal(), placeholderPrefix) {
			r.fail("SUPERTOKENS_API_KEY", "is still the example placeholder")
		}
	}
}

const maxAdminSessionAge = 7 * 24 * time.Hour

func checkAdmin(r *reader, a Admin, prod bool) {
	if !a.Enabled() {
		for key, v := range map[string]string{"OIDC_CLIENT_ID": a.ClientID, "ADMIN_EMAIL_DOMAIN": a.EmailDomain, "OIDC_HOSTED_DOMAIN": a.HostedDomain, "ADMIN_API_HOST": a.APIHost} {
			if v != "" {
				r.fail(key, "is set but OIDC_ISSUER is empty, so admin sign-in is disabled")
			}
		}
		return
	}
	u, err := url.Parse(a.Issuer)
	if err != nil || u.Host == "" || u.Path != "" && u.Path != "/" || u.RawQuery != "" {
		r.fail("OIDC_ISSUER", "%q must be the issuer URL, e.g. https://accounts.google.com", a.Issuer)
	} else if u.Scheme != "https" && (prod || !isLoopback(u.Hostname())) {
		r.fail("OIDC_ISSUER", "must use https (http is allowed only for localhost outside production)")
	}
	r.require("OIDC_CLIENT_ID", a.ClientID != "")
	r.require("OIDC_CLIENT_SECRET", a.ClientSecret.IsSet())
	r.require("ADMIN_EMAIL_DOMAIN", a.EmailDomain != "")
	if a.EmailDomain != "" && (strings.ContainsAny(a.EmailDomain, "@/ ") || !strings.Contains(a.EmailDomain, ".")) {
		r.fail("ADMIN_EMAIL_DOMAIN", "%q must be a bare domain like coniungolabs.com", a.EmailDomain)
	}
	if a.HostedDomain != "" && a.HostedDomain != a.EmailDomain {
		r.fail("OIDC_HOSTED_DOMAIN", "must equal ADMIN_EMAIL_DOMAIN")
	}
	if a.SessionMaxAge > maxAdminSessionAge {
		r.fail("ADMIN_SESSION_MAX_AGE", "must be at most %s", maxAdminSessionAge)
	}
	if prod {
		r.require("ADMIN_API_HOST", a.APIHost != "")
	}
	if a.APIHost != "" && (strings.ContainsAny(a.APIHost, "/@ ") || strings.Contains(a.APIHost, "://")) {
		r.fail("ADMIN_API_HOST", "%q must be a bare host like admin-api.example.com", a.APIHost)
	}
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

var (
	emailProviders = []string{ProviderLog, ProviderPostmark}
	smsProviders   = []string{ProviderLog, ProviderTermii}
)

func checkDelivery(r *reader, d Delivery, prod bool) {
	r.oneOf("EMAIL_PROVIDER", d.EmailProvider, emailProviders...)
	r.oneOf("SMS_PROVIDER", d.SMSProvider, smsProviders...)
	if d.EmailFrom != "" {
		if _, err := mail.ParseAddress(d.EmailFrom); err != nil {
			r.fail("EMAIL_FROM", "%q is not a valid address, e.g. Brand <hello@example.com>", d.EmailFrom)
		}
	}
	if d.EmailProvider == ProviderPostmark {
		r.require("POSTMARK_SERVER_TOKEN", d.PostmarkServerToken.IsSet())
		r.require("EMAIL_FROM", d.EmailFrom != "")
	}
	if d.SMSProvider == ProviderTermii {
		r.require("TERMII_API_KEY", d.TermiiAPIKey.IsSet())
		r.require("TERMII_BASE_URL", d.TermiiBaseURL != "")
		if d.TermiiBaseURL != "" && checkOrigin(r, "TERMII_BASE_URL", d.TermiiBaseURL) {
			checkHTTPS(r, "TERMII_BASE_URL", d.TermiiBaseURL)
		}
		if !validSenderID(d.SMSSender) {
			r.fail("SMS_SENDER", "%q must be the Termii sender ID: 3 to 11 letters, digits or spaces", d.SMSSender)
		}
		r.oneOf("TERMII_CHANNEL", d.TermiiChannel, "dnd", "generic")
		if prod && d.TermiiChannel != "dnd" {
			r.fail("TERMII_CHANNEL", "must be dnd in production: generic is for promotions and does not reach numbers on Do-Not-Disturb")
		}
	}
	if prod {
		if d.EmailProvider == ProviderLog {
			r.fail("EMAIL_PROVIDER", "log only prints codes and is not allowed in production")
		}
		if d.SMSProvider == ProviderLog {
			r.fail("SMS_PROVIDER", "log only prints codes and is not allowed in production")
		}
		r.require("EMAIL_FROM", d.EmailFrom != "")
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

func validSenderID(s string) bool {
	if len(s) < 3 || len(s) > 11 || strings.TrimSpace(s) != s {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != ' ' {
			return false
		}
	}
	return true
}
