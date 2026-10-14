package requestid

import (
	"strings"
	"testing"
)

func TestValid(t *testing.T) {
	tests := map[string]bool{
		"":                      false,
		"abc-XYZ_09":            true,
		strings.Repeat("a", 64): true,
		strings.Repeat("a", 65): false,
		"has space":             false,
		"new\nline":             false,
		"semi;colon":            false,
		"é":                     false,
		"`":                     false,
		"{":                     false,
		"@":                     false,
		"[":                     false,
		"/":                     false,
		":":                     false,
	}
	for id, want := range tests {
		if got := Valid(id); got != want {
			t.Errorf("Valid(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestNewIsValid(t *testing.T) {
	if id := New(); !Valid(id) {
		t.Fatalf("New() produced invalid id %q", id)
	}
}
