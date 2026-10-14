package config

import (
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
)

type reader struct {
	lookup func(string) (string, bool)
	errs   []error
}

func (r *reader) fail(key, format string, args ...any) {
	r.errs = append(r.errs, fmt.Errorf("%s: %s", key, fmt.Sprintf(format, args...)))
}

func (r *reader) raw(key string) string {
	v, _ := r.lookup(key)
	return strings.TrimSpace(v)
}

func (r *reader) string(key, fallback string) string {
	if v := r.raw(key); v != "" {
		return v
	}
	return fallback
}

func (r *reader) secret(key string) Secret {
	return Secret(r.raw(key))
}

func (r *reader) int(key string, fallback int) int {
	v := r.raw(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		r.fail(key, "%q is not a valid integer", v)
		return fallback
	}
	return n
}

func (r *reader) int32(key string, fallback, lo, hi int32) int32 {
	v := r.raw(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil || int32(n) < lo || int32(n) > hi {
		r.fail(key, "%q must be an integer between %d and %d", v, lo, hi)
		return fallback
	}
	return int32(n)
}

func (r *reader) duration(key string, fallback time.Duration) time.Duration {
	v := r.raw(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		r.fail(key, "%q is not a valid positive duration (e.g. 15s, 2m)", v)
		return fallback
	}
	return d
}

func (r *reader) durationOrZero(key string, fallback time.Duration) time.Duration {
	v := r.raw(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		r.fail(key, "%q is not a valid duration (e.g. 0s, 5s)", v)
		return fallback
	}
	return d
}

func (r *reader) list(key string, fallback []string) []string {
	v := r.raw(key)
	if v == "" {
		return fallback
	}
	var out []string
	for part := range strings.SplitSeq(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (r *reader) logLevel(key string, fallback slog.Level) slog.Level {
	v := r.raw(key)
	if v == "" {
		return fallback
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToLower(v))); err != nil {
		r.fail(key, "%q is not a valid log level (debug, info, warn, error)", v)
		return fallback
	}
	return level
}

func (r *reader) oneOf(key, value string, choices ...string) {
	if !slices.Contains(choices, value) {
		r.fail(key, "%q is not one of: %s", value, strings.Join(choices, ", "))
	}
}

func (r *reader) require(key string, set bool) {
	if !set {
		r.fail(key, "is required but not set")
	}
}
