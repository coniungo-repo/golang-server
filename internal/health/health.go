package health

import (
	"context"
	"net/http"
	"time"

	"github.com/coniungo-repo/golang-server.git/internal/platform/respond"
	"github.com/coniungo-repo/golang-server.git/internal/platform/route"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RegisterRoutes(mux route.Mux, db *pgxpool.Pool) {
	h := &handler{db: db}

	mux.Handle("GET /health", http.HandlerFunc(h.health))
}

type handler struct {
	db *pgxpool.Pool
}

func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "Database is unreachable")
		return
	}

	respond.JSON(w, r, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}
