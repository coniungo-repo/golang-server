package auth

import (
	"net/http"

	"github.com/coniungo-repo/golang-server/internal/platform/respond"
	"github.com/coniungo-repo/golang-server/internal/platform/route"
)

// Register mounts the session-protected account routes.
func (s *Service) Register(mux route.Mux) {
	mux.Handle("GET /me", s.RequireSession(http.HandlerFunc(me)))
}

type meResponse struct {
	UserID string `json:"user_id"`
}

func me(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, r, http.StatusOK, meResponse{UserID: UserID(r.Context())})
}
