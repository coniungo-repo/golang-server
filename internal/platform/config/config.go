package config

import (
	"log/slog"
	"net"
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
	RequestTimeout    time.Duration
}

func (s Server) Addr() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

type Database struct {
	URL          Secret
	MaxOpenConns int
	MaxIdleConns int
	ConnMaxIdle  time.Duration
}

type Log struct {
	Level slog.Level
}

type CORS struct {
	AllowedOrigins []string
}
