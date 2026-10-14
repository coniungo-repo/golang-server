package route

import (
	"net/http"
	"sync"
)

type Mux interface {
	Handle(pattern string, handler http.Handler)
}

type Recorder struct {
	mux      Mux
	mu       sync.Mutex
	patterns []string
}

func NewRecorder(m Mux) *Recorder { return &Recorder{mux: m} }

func (r *Recorder) Handle(pattern string, h http.Handler) {
	r.mu.Lock()
	r.patterns = append(r.patterns, pattern)
	r.mu.Unlock()

	r.mux.Handle(pattern, h)
}

func (r *Recorder) Patterns() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.patterns...)
}
