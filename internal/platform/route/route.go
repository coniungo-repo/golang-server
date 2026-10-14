package route

import (
	"net/http"
	"strings"
)

type Mux interface {
	Handle(pattern string, handler http.Handler)
}

type prefixed struct {
	mux    Mux
	prefix string
}

// Prefix returns a Mux that registers every pattern under prefix, keeping any method, e.g. "GET /things" becomes "GET /api/v1/things".
func Prefix(mux Mux, prefix string) Mux {
	return prefixed{mux: mux, prefix: strings.TrimSuffix(prefix, "/")}
}

func (p prefixed) Handle(pattern string, h http.Handler) {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok {
		p.mux.Handle(p.prefix+pattern, h)
		return
	}
	p.mux.Handle(method+" "+p.prefix+strings.TrimSpace(path), h)
}
