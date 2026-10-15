// Package phone parses user-entered phone numbers into canonical E.164 form.
package phone

import (
	"errors"
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nyaruka/phonenumbers/v2"
)

const maxInputLen = 64

var (
	ErrEmpty              = errors.New("phone: empty")
	ErrTooLong            = errors.New("phone: input too long")
	ErrInvalidCharacters  = errors.New("phone: invalid characters")
	ErrMissingCountryCode = errors.New("phone: missing country code")
	ErrInvalidCountryCode = errors.New("phone: invalid country code")
	ErrTooShort           = errors.New("phone: too short")
	ErrTooManyDigits      = errors.New("phone: too many digits")
	ErrExtension          = errors.New("phone: extensions not allowed")
	ErrInvalid            = errors.New("phone: not a valid number")
	ErrRegionNotAllowed   = errors.New("phone: region not allowed")
	ErrTypeNotAllowed     = errors.New("phone: number type not allowed")
)

var messages = map[error]string{
	ErrEmpty:              "Enter your phone number.",
	ErrTooLong:            "That phone number is too long. Check it and try again.",
	ErrInvalidCharacters:  "Phone numbers can only contain digits, spaces, +, -, ( and ).",
	ErrMissingCountryCode: "Add your country code, starting with +.",
	ErrInvalidCountryCode: "That country code isn't right. Check it and try again.",
	ErrTooShort:           "That phone number is too short. Check it and try again.",
	ErrTooManyDigits:      "That phone number has too many digits. Check it and try again.",
	ErrExtension:          "Leave out the extension and try again.",
	ErrInvalid:            "That isn't a valid phone number. Check it and try again.",
	ErrRegionNotAllowed:   "We don't support phone numbers from that country yet.",
	ErrTypeNotAllowed:     "Use a mobile number that can receive texts.",
}

// Message returns copy safe to show the user for an error from Parse.
func Message(err error) string {
	for target, msg := range messages {
		if errors.Is(err, target) {
			return msg
		}
	}
	return messages[ErrInvalid]
}

// Number is a validated phone number. String and LogValue mask it; call E164 for storage and delivery.
type Number struct {
	e164        string
	region      string
	countryCode int
	kind        Type
}

func (n Number) E164() string         { return n.e164 }
func (n Number) Region() string       { return n.region }
func (n Number) CountryCode() int     { return n.countryCode }
func (n Number) Type() Type           { return n.kind }
func (n Number) IsZero() bool         { return n.e164 == "" }
func (n Number) String() string       { return mask(n.e164) }
func (n Number) LogValue() slog.Value { return slog.StringValue(mask(n.e164)) }

func mask(e164 string) string {
	const visibleHead, visibleTail = 4, 2
	if len(e164) <= visibleHead+visibleTail {
		return strings.Repeat("*", len(e164))
	}
	return e164[:visibleHead] + strings.Repeat("*", len(e164)-visibleHead-visibleTail) + e164[len(e164)-visibleTail:]
}

// Validator parses numbers under a fixed policy. It is safe for concurrent use.
type Validator struct {
	defaultRegion string
	regions       map[string]bool
	types         map[Type]bool
}

type Option func(*Validator) error

// WithDefaultRegion sets the ISO 3166-1 alpha-2 region assumed when a number has no +country code.
func WithDefaultRegion(region string) Option {
	return func(v *Validator) error {
		r, err := normalizeRegion(region)
		if err != nil {
			return err
		}
		v.defaultRegion = r
		return nil
	}
}

// WithAllowedRegions restricts numbers to the given regions. Without it, every region is accepted.
func WithAllowedRegions(regions ...string) Option {
	return func(v *Validator) error {
		v.regions = make(map[string]bool, len(regions))
		for _, region := range regions {
			r, err := normalizeRegion(region)
			if err != nil {
				return err
			}
			v.regions[r] = true
		}
		return nil
	}
}

// WithTypes sets the accepted number types. The default is Mobile and FixedLineOrMobile.
func WithTypes(types ...Type) Option {
	return func(v *Validator) error {
		if len(types) == 0 {
			return errors.New("phone: WithTypes needs at least one type")
		}
		v.types = make(map[Type]bool, len(types))
		for _, t := range types {
			v.types[t] = true
		}
		return nil
	}
}

// AnyType accepts every valid number regardless of type.
func AnyType() Option {
	return func(v *Validator) error {
		v.types = nil
		return nil
	}
}

func New(opts ...Option) (*Validator, error) {
	v := &Validator{types: map[Type]bool{Mobile: true, FixedLineOrMobile: true}}
	for _, opt := range opts {
		if err := opt(v); err != nil {
			return nil, err
		}
	}
	if v.defaultRegion != "" && v.regions != nil && !v.regions[v.defaultRegion] {
		return nil, errors.New("phone: default region must be one of the allowed regions")
	}
	return v, nil
}

// Parse validates raw user input and returns the number in canonical form. Errors wrap the package's Err values; use Message for user-facing copy.
func (v *Validator) Parse(raw string) (Number, error) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return Number{}, ErrEmpty
	}
	if len(input) > maxInputLen {
		return Number{}, ErrTooLong
	}
	if !utf8.ValidString(input) || !allowedCharacters(input) {
		return Number{}, ErrInvalidCharacters
	}
	if !internationalForm(input) && v.defaultRegion == "" {
		return Number{}, ErrMissingCountryCode
	}

	pn := &phonenumbers.PhoneNumber{}
	if err := phonenumbers.ParseToNumber(input, v.defaultRegion, pn); err != nil {
		return Number{}, parseError(err)
	}
	if pn.GetExtension() != "" {
		return Number{}, ErrExtension
	}

	switch phonenumbers.IsPossibleNumberWithReason(pn) {
	case phonenumbers.IS_POSSIBLE:
	case phonenumbers.INVALID_COUNTRY_CODE:
		return Number{}, ErrInvalidCountryCode
	case phonenumbers.TOO_SHORT:
		return Number{}, ErrTooShort
	case phonenumbers.TOO_LONG:
		return Number{}, ErrTooManyDigits
	default:
		return Number{}, ErrInvalid
	}
	if !phonenumbers.IsValidNumber(pn) {
		return Number{}, ErrInvalid
	}

	region := phonenumbers.GetRegionCodeForNumber(pn)
	if v.regions != nil && !v.regions[region] {
		return Number{}, ErrRegionNotAllowed
	}
	kind := typeOf(phonenumbers.GetNumberType(pn))
	if v.types != nil && !v.types[kind] {
		return Number{}, ErrTypeNotAllowed
	}

	return Number{
		e164:        phonenumbers.Format(pn, phonenumbers.E164),
		region:      region,
		countryCode: int(pn.GetCountryCode()),
		kind:        kind,
	}, nil
}

// Valid reports whether raw passes Parse.
func (v *Validator) Valid(raw string) bool {
	_, err := v.Parse(raw)
	return err == nil
}

func allowedCharacters(s string) bool {
	plusSeen := false
	for i, c := range s {
		switch {
		case unicode.IsDigit(c):
		case c == ' ', c == '-', c == '.', c == '(', c == ')', c == '/', c == '\u00a0':
		case c == '+' || c == '\uff0b':
			if plusSeen || strings.TrimSpace(s[:i]) != "" {
				return false
			}
			plusSeen = true
		default:
			return false
		}
	}
	return true
}

func internationalForm(s string) bool {
	return strings.HasPrefix(s, "+") || strings.HasPrefix(s, "\uff0b")
}

func parseError(err error) error {
	switch {
	case errors.Is(err, phonenumbers.ErrInvalidCountryCode):
		return ErrInvalidCountryCode
	case errors.Is(err, phonenumbers.ErrTooShortNSN), errors.Is(err, phonenumbers.ErrTooShortAfterIDD):
		return ErrTooShort
	case errors.Is(err, phonenumbers.ErrNumTooLong):
		return ErrTooLong
	default:
		return ErrInvalid
	}
}

func normalizeRegion(region string) (string, error) {
	r := strings.ToUpper(strings.TrimSpace(region))
	if !phonenumbers.GetSupportedRegions()[r] {
		return "", errors.New("phone: unsupported region " + region)
	}
	return r, nil
}
