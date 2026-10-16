package auth

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/coniungo-repo/golang-server/internal/platform/metrics"
	"github.com/coniungo-repo/golang-server/internal/platform/middleware"
	"github.com/coniungo-repo/golang-server/internal/platform/respond"
	"github.com/coniungo-repo/golang-server/internal/users"
	"github.com/supertokens/supertokens-golang/recipe/session"
)

const (
	signInAgainMessage  = "Sign in again to continue."
	notAllowedMessage   = "You don't have access to this."
	adminSignInAgainMsg = "Your admin session has expired. Sign in again."
)

type Permission string

const (
	PermManageAdminInvites Permission = "admin_invites:manage"
	PermManageUserStatus   Permission = "user_status:manage"
	PermDeleteUsers        Permission = "users:delete"
)

var rolePermissions = map[users.Role][]Permission{
	users.RoleAdmin: {PermManageAdminInvites, PermManageUserStatus, PermDeleteUsers},
}

func (p Principal) Can(perm Permission) bool {
	for _, r := range p.Roles {
		for _, granted := range rolePermissions[r] {
			if granted == perm {
				return true
			}
		}
	}
	return false
}

// RequireSession rejects requests without a valid session, over the per-user rate limit, from a suspended or deleted user, or from a sign-in method since removed, and puts the Principal on the request context. The rate limit runs first so refused requests cost no query. Access tokens are verified without a core call, so the database check is what makes suspension immediate.
func (s *Service) RequireSession(next http.Handler) http.Handler {
	return session.VerifySession(nil, func(w http.ResponseWriter, r *http.Request) {
		p, stUserID, ok := principalFromSession(r.Context())
		if !ok {
			respond.Error(w, r, http.StatusUnauthorized, signInAgainMessage)
			return
		}
		middleware.SetUserID(r.Context(), p.UserID)
		if ok, wait := s.deps.UserLimit.Allow(p.UserID); !ok {
			s.deps.Log.WarnContext(r.Context(), "rate limited", "limit", metrics.LimitUser, "retry_after", wait)
			s.deps.Metrics.RateLimited(metrics.LimitUser)
			respond.TooManyRequests(w, r, wait)
			return
		}
		active, err := s.deps.Users.IsActive(r.Context(), p.UserID, stUserID)
		if err != nil {
			respond.ServerError(w, r, err)
			return
		}
		if !active {
			respond.Error(w, r, http.StatusUnauthorized, suspendedMessage)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

// RequireRole allows signed-in users holding role. Roles come from the session, so revoke sessions when roles change.
func (s *Service) RequireRole(role users.Role, next http.Handler) http.Handler {
	return s.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, _ := FromContext(r.Context()); !p.Has(role) {
			respond.Error(w, r, http.StatusForbidden, notAllowedMessage)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// RequirePermission allows signed-in users whose roles grant perm.
func (s *Service) RequirePermission(perm Permission, next http.Handler) http.Handler {
	return s.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, _ := FromContext(r.Context()); !p.Can(perm) {
			respond.Error(w, r, http.StatusForbidden, notAllowedMessage)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// RequireAdmin allows only admin-host requests from an OpenID Connect session younger than the admin maximum, re-checking the user and role in the database every time.
func (s *Service) RequireAdmin(perm Permission, next http.Handler) http.Handler {
	inner := s.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := FromContext(r.Context())
		if p.Method != MethodOIDC || !p.Has(users.RoleAdmin) || !p.Can(perm) {
			respond.Error(w, r, http.StatusForbidden, notAllowedMessage)
			return
		}
		if s.deps.Clock.Since(p.AuthTime) > s.deps.Admin.SessionMaxAge {
			respond.Error(w, r, http.StatusUnauthorized, adminSignInAgainMsg)
			return
		}
		active, err := s.deps.Users.IsActiveAdmin(r.Context(), p.UserID)
		if err != nil {
			respond.ServerError(w, r, err)
			return
		}
		if !active {
			respond.Error(w, r, http.StatusForbidden, notAllowedMessage)
			return
		}
		next.ServeHTTP(w, r)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.onAdminHost(r) {
			respond.Error(w, r, http.StatusNotFound, "We couldn't find that. Check the link and try again.")
			return
		}
		inner.ServeHTTP(w, r)
	})
}

func (s *Service) onAdminHost(r *http.Request) bool {
	if s.deps.Admin.APIHost == "" {
		return true
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.EqualFold(host, s.deps.Admin.APIHost)
}
