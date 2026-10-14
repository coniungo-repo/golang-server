package config

import (
	"log/slog"
)

// Secret hides its value from fmt, slog and encoders. Reveal returns the real value.
type Secret string

// Reveal returns the actual underlying string. Use this only when
// passing the value to a driver (DB, API client, etc).
func (s Secret) Reveal() string { return string(s) }

// IsSet returns true if the secret is not an empty string.
func (s Secret) IsSet() bool { return s != "" }

// String implements fmt.Stringer to prevent accidental leakage in Printf %s and %v.
func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return "[redacted]"
}

// GoString implements fmt.GoStringer to prevent leakage when using %#v (Go-syntax representation).
func (s Secret) GoString() string {
	return s.String()
}

// LogValue implements slog.LogValuer to ensure slog always redacts this type.
func (s Secret) LogValue() slog.Value {
	return slog.StringValue(s.String())
}

// MarshalText implements encoding.TextMarshaler for YAML and TOML encoders.
func (s Secret) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// MarshalJSON implements json.Marshaler for the standard encoding/json package.
// This is critical because MarshalJSON takes precedence over MarshalText.
func (s Secret) MarshalJSON() ([]byte, error) {
	// We wrap in quotes because JSON strings must be quoted.
	return []byte(`"` + s.String() + `"`), nil
}
