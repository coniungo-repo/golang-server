package auth

import (
	"net/http"

	"github.com/coniungo-repo/golang-server/internal/platform/respond"
	"github.com/coniungo-repo/golang-server/internal/platform/route"
	"github.com/coniungo-repo/golang-server/internal/users"
)

// Register mounts the account routes and, when admin sign-in is configured, the admin routes.
func (s *Service) Register(mux route.Mux) {
	mux.Handle("GET /me", s.RequireSession(http.HandlerFunc(me)))
	if s.deps.Admin.Enabled() {
		s.registerAdmin(mux)
	}
}

type meResponse struct {
	UserID string       `json:"user_id"`
	Roles  []users.Role `json:"roles"`
	Method string       `json:"sign_in_method"`
}

func me(w http.ResponseWriter, r *http.Request) {
	p, _ := FromContext(r.Context())
	if p.Roles == nil {
		p.Roles = []users.Role{}
	}
	respond.JSON(w, r, http.StatusOK, meResponse{UserID: p.UserID, Roles: p.Roles, Method: p.Method})
}
