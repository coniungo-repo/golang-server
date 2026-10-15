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
	Security Security
	Phone    Phone
}

type App struct {
	Name        string
	Environment Environment
	Version     string
}

func (a App) IsProduction() bool { return a.Environment == Production }

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
}

func (s Server) Addr() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

type Database struct {
	URL         Secret
	MaxConns    int32
	MinConns    int32
	ConnMaxIdle time.Duration
}

type Auth struct {
	ConnectionURI   string
	APIKey          Secret
	AppName         string
	APIDomain       string
	APIBasePath     string
	WebsiteDomains  []string
	WebsiteBasePath string
	RateLimits      RateLimits
}

// RateWindow allows Limit attempts in any rolling Period.
type RateWindow struct {
	Limit  int64
	Period time.Duration
}

// RateLimits caps sign-in code sends. Every window applies; the strictest one wins.
type RateLimits struct {
	Destination []RateWindow
	ClientIP    []RateWindow
}

// Retention is the longest window, which is how long attempts must be kept.
func (l RateLimits) Retention() time.Duration {
	var longest time.Duration
	for _, w := range append(slices.Clone(l.Destination), l.ClientIP...) {
		longest = max(longest, w.Period)
	}
	return longest
}

type Log struct {
	Level slog.Level
}

type CORS struct {
	AllowedOrigins []string
}

type Security struct {
	FingerprintSecret Secret
}

type Phone struct {
	DefaultRegion  string
	AllowedRegions []string
}

const ProviderLog = "log"

type Delivery struct {
	EmailProvider string
	EmailFrom     string
	SMSProvider   string
	SMSSender     string
}
