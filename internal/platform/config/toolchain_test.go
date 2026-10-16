package config

import (
	"os"
	"regexp"
	"testing"
)

var (
	goModToolchain = regexp.MustCompile(`(?m)^toolchain go([0-9.]+)$`)
	dockerGo       = regexp.MustCompile(`(?m)^FROM golang:([0-9.]+)-`)
)

// TestDockerfileMatchesToolchain keeps the image's compiler the same as the one CI tests with, which go.mod's toolchain line selects. Dependabot bumps the Dockerfile; bump go.mod with it.
func TestDockerfileMatchesToolchain(t *testing.T) {
	mod, err := os.ReadFile("../../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	docker, err := os.ReadFile("../../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	want := goModToolchain.FindSubmatch(mod)
	got := dockerGo.FindSubmatch(docker)
	if want == nil || got == nil {
		t.Fatal("go.mod needs a toolchain line and the Dockerfile a golang:<version>-… base image")
	}
	if string(got[1]) != string(want[1]) {
		t.Fatalf("Dockerfile builds with Go %s but go.mod's toolchain is go%s; set both to the same version", got[1], want[1])
	}
}
