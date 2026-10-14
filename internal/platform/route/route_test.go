package route

import (
	"net/http"
	"slices"
	"testing"
)

type recordingMux []string

func (m *recordingMux) Handle(pattern string, _ http.Handler) { *m = append(*m, pattern) }

func TestPrefix(t *testing.T) {
	var m recordingMux
	p := Prefix(&m, "/api/v1/")
	p.Handle("GET /things/{id}", nil)
	p.Handle("/things", nil)

	want := recordingMux{"GET /api/v1/things/{id}", "/api/v1/things"}
	if !slices.Equal(m, want) {
		t.Fatalf("got %v, want %v", m, want)
	}
}
