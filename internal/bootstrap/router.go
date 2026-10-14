package bootstrap

import (
	"net/http"

	"github.com/coniungo-repo/golang-server.git/internal/health"
	"github.com/coniungo-repo/golang-server.git/internal/platform/middleware"
	"github.com/coniungo-repo/golang-server.git/internal/platform/respond"
	"github.com/coniungo-repo/golang-server.git/internal/platform/route"
)

const (
	maxBodyBytes = 1 << 20
	apiPrefix    = "/api/v1"
)

func newRouter(deps *dependencies) http.Handler {

	apiMux := http.NewServeMux()
	recorder := route.NewRecorder(apiMux)
	registerAPI(recorder, deps)
	apiMux.HandleFunc("/", notFound)

	var apiHandler = http.StripPrefix(apiPrefix, apiMux)

	apiHandler = middleware.Chain(
		apiHandler,
		middleware.RequestID,
		middleware.Log,
		middleware.Routes(apiMux),
		middleware.Recover,
		middleware.SecureHeaders,
		middleware.CORS(deps.Config.CORS.AllowedOrigins, []string{"Content-Type", "Authorization", "X-Request-ID"}),
		middleware.MaxBytes(maxBodyBytes),
		middleware.Timeout(deps.Config.Server.RequestTimeout),
	)

	root := http.NewServeMux()
	registerRoot(root, deps)

	root.Handle(apiPrefix+"/", apiHandler)

	root.HandleFunc("/", notFound)

	return root
}
func registerRoot(mux route.Mux, deps *dependencies) {
	health.RegisterRoutes(mux, deps.DB)
	if !deps.Config.App.IsProduction() {
	}
}

func registerAPI(mux route.Mux, deps *dependencies) {
}

func notFound(w http.ResponseWriter, r *http.Request) {
	respond.Error(w, r, http.StatusNotFound, "We couldn't find that. Check the link and try again.")
}
