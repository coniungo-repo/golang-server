package phone

import "github.com/nyaruka/phonenumbers"

type Type int

const (
	Unknown Type = iota
	Mobile
	FixedLine
	FixedLineOrMobile
	TollFree
	PremiumRate
	SharedCost
	VoIP
	Personal
	Pager
	UAN
	Voicemail
)

var typeNames = [...]string{"unknown", "mobile", "fixed_line", "fixed_line_or_mobile", "toll_free", "premium_rate", "shared_cost", "voip", "personal", "pager", "uan", "voicemail"}

func (t Type) String() string {
	if t < 0 || int(t) >= len(typeNames) {
		return typeNames[Unknown]
	}
	return typeNames[t]
}

func typeOf(t phonenumbers.PhoneNumberType) Type {
	switch t {
	case phonenumbers.MOBILE:
		return Mobile
	case phonenumbers.FIXED_LINE:
		return FixedLine
	case phonenumbers.FIXED_LINE_OR_MOBILE:
		return FixedLineOrMobile
	case phonenumbers.TOLL_FREE:
		return TollFree
	case phonenumbers.PREMIUM_RATE:
		return PremiumRate
	case phonenumbers.SHARED_COST:
		return SharedCost
	case phonenumbers.VOIP:
		return VoIP
	case phonenumbers.PERSONAL_NUMBER:
		return Personal
	case phonenumbers.PAGER:
		return Pager
	case phonenumbers.UAN:
		return UAN
	case phonenumbers.VOICEMAIL:
		return Voicemail
	default:
		return Unknown
	}
}
