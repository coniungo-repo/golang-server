package config

import (
	"log/slog"
	"net"
	"slices"
	"strconv"
	"time"
)

type Environment string

const (
	Development Environment = "development"
	Test        Environment = "test"
	Production  Environment = "production"
)

type Config struct {
	App      App
	Server   Server
	Log      Log
	Database Database
	CORS     CORS
	Auth     Auth
	Delivery Delivery
	Admin    Admin
	Security Security
	Phone    Phone
	Docs     Docs
	Limits   Limits
}

// Limits caps general request rates per client IP on every route but health checks, and per signed-in user.
type Limits struct {
	ClientIP []RateWindow
	User     []RateWindow
}

type App struct {
	Name        string
	Environment Environment
	Version     string
}

func (a App) IsProduction() bool { return a.Environment == Production }

// Docs serves the OpenAPI description and Swagger UI. On by default outside production.
type Docs struct {
	Enabled bool
}

type Server struct {
	Host              string
	Port              int
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	DrainDelay        time.Duration
	RequestTimeout    time.Duration
	RequestIDHeader   string
	ClientIPHeader    string
	MaxHeaderBytes    int
	MetricsAddr       string
}

func (s Server) Addr() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

type Database struct {
	URL               Secret
	MaxConns          int32
	MinConns          int32
	ConnMaxIdle       time.Duration
	ConnectTimeout    time.Duration
	HealthCheckPeriod time.Duration
	ReadyTimeout      time.Duration
	StatementTimeout  time.Duration
}

type Auth struct {
	ConnectionURI   string
	APIKey          Secret
	AppName         string
	APIDomain       string
	APIBasePath     string
	WebsiteDomains  []string
	WebsiteBasePath string
	CoreTimeout     time.Duration
	RateLimits      RateLimits
}

// RateWindow allows Limit attempts in any rolling Period.
type RateWindow struct {
	Limit  int64
	Period time.Duration
}

// RateLimits caps sign-in code sends. Every window applies; the strictest one wins. SMSTotal caps texts across all numbers and clients, bounding SMS spend.
type RateLimits struct {
	Destination []RateWindow
	ClientIP    []RateWindow
	SMSTotal    []RateWindow
}

// Retention is the longest window, which is how long attempts must be kept.
func (l RateLimits) Retention() time.Duration {
	var longest time.Duration
	for _, w := range slices.Concat(l.Destination, l.ClientIP, l.SMSTotal) {
		longest = max(longest, w.Period)
	}
	return longest
}

type Log struct {
	Level slog.Level
}

type CORS struct {
	AllowedOrigins []string
	MaxAge         time.Duration
}

type Security struct {
	FingerprintSecret Secret
	BotCheck          string
	TurnstileSecret   Secret
	BotCheckTimeout   time.Duration
}

type Phone struct {
	DefaultRegion  string
	AllowedRegions []string
}

const (
	BotCheckOff       = "off"
	BotCheckTurnstile = "turnstile"
)

const (
	ProviderLog      = "log"
	ProviderPostmark = "postmark"
	ProviderTermii   = "termii"
)

type Delivery struct {
	EmailProvider string
	EmailFrom     string
	SMSProvider   string
	SMSSender     string
	Timeout       time.Duration

	PostmarkServerToken Secret
	PostmarkStream      string

	TermiiAPIKey  Secret
	TermiiBaseURL string
	TermiiChannel string
}

// Admin configures staff sign-in through an OpenID Connect provider such as Google Workspace. An empty Issuer disables it.
type Admin struct {
	Issuer        string
	ClientID      string
	ClientSecret  Secret
	HostedDomain  string
	EmailDomain   string
	APIHost       string
	SessionMaxAge time.Duration
}

func (a Admin) Enabled() bool { return a.Issuer != "" }
