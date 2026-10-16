// Package docs serves the OpenAPI description and a Swagger UI to explore it. Enable it only where the API surface may be public.
package docs

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/coniungo-repo/golang-server/internal/platform/route"
)

//go:embed swaggerui
var assets embed.FS

const csp = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// Register serves spec at /openapi.yaml and Swagger UI at /docs/.
func Register(mux route.Mux, spec []byte) {
	ui, err := fs.Sub(assets, "swaggerui")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /openapi.yaml", withCSP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(spec)
	})))
	mux.Handle("GET /docs", http.RedirectHandler("/docs/", http.StatusMovedPermanently))
	mux.Handle("GET /docs/", withCSP(http.StripPrefix("/docs/", http.FileServerFS(ui))))
}

func withCSP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}
