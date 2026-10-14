package config

import "log/slog"

// Secret hides its value from fmt, slog and encoders. Call Reveal only where the raw value is needed.
type Secret string

func (s Secret) Reveal() string { return string(s) }

func (s Secret) IsSet() bool { return s != "" }

func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return "[redacted]"
}

func (s Secret) GoString() string { return s.String() }

func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }

func (s Secret) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + s.String() + `"`), nil }
