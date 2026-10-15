package bootstrap

import (
	"testing"

	"github.com/coniungo-repo/golang-server/internal/platform/config"
)

func TestNewPhoneValidator(t *testing.T) {
	v, err := newPhoneValidator(config.Phone{DefaultRegion: "NG", AllowedRegions: []string{"NG", "GH"}})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Valid("0803 123 4567") {
		t.Fatal("expected local Nigerian mobile to be valid")
	}
	if _, err := newPhoneValidator(config.Phone{DefaultRegion: "XX"}); err == nil {
		t.Fatal("expected unsupported region to fail startup")
	}
}
