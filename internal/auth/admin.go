package auth

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/coniungo-repo/golang-server/internal/platform/request"
	"github.com/coniungo-repo/golang-server/internal/platform/respond"
	"github.com/coniungo-repo/golang-server/internal/platform/route"
	"github.com/coniungo-repo/golang-server/internal/users"
)

func (s *Service) registerAdmin(mux route.Mux) {
	mux.Handle("GET /admin/invites", s.RequireAdmin(PermManageAdminInvites, http.HandlerFunc(s.listInvites)))
	mux.Handle("POST /admin/invites", s.RequireAdmin(PermManageAdminInvites, http.HandlerFunc(s.createInvite)))
	mux.Handle("POST /admin/invites/{id}/revoke", s.RequireAdmin(PermManageAdminInvites, http.HandlerFunc(s.revokeInvite)))
	mux.Handle("POST /admin/users/{id}/suspend", s.RequireAdmin(PermManageUserStatus, http.HandlerFunc(s.setStatus(users.StatusSuspended))))
	mux.Handle("POST /admin/users/{id}/reactivate", s.RequireAdmin(PermManageUserStatus, http.HandlerFunc(s.setStatus(users.StatusActive))))
}

type inviteRequest struct {
	Email string `json:"email"`
}

type invitesResponse struct {
	Invites []users.Invite `json:"invites"`
}

func (s *Service) listInvites(w http.ResponseWriter, r *http.Request) {
	invites, err := s.deps.Users.ListAdminInvites(r.Context())
	if err != nil {
		respond.ServerError(w, r, err)
		return
	}
	respond.JSON(w, r, http.StatusOK, invitesResponse{Invites: invites})
}

func (s *Service) createInvite(w http.ResponseWriter, r *http.Request) {
	var body inviteRequest
	if err := request.Decode(w, r, &body); err != nil {
		s.decodeError(w, r, err)
		return
	}
	actor := UserID(r.Context())
	inv, err := s.deps.Users.CreateAdminInvite(r.Context(), body.Email, &actor)
	switch {
	case errors.Is(err, users.ErrInvalidEmail):
		respond.Error(w, r, http.StatusUnprocessableEntity, "Invite a staff email on @"+s.deps.Admin.EmailDomain+".")
	case errors.Is(err, users.ErrInviteExists):
		respond.Error(w, r, http.StatusConflict, "That email already has an invite.")
	case err != nil:
		respond.ServerError(w, r, err)
	default:
		respond.JSON(w, r, http.StatusCreated, inv)
	}
}

func (s *Service) revokeInvite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		respond.Error(w, r, http.StatusNotFound, "We couldn't find that invite.")
		return
	}
	revokedUser, err := s.deps.Users.RevokeAdminInvite(r.Context(), id, UserID(r.Context()))
	switch {
	case errors.Is(err, users.ErrNotFound):
		respond.Error(w, r, http.StatusNotFound, "We couldn't find that invite.")
		return
	case errors.Is(err, users.ErrSelfAction):
		respond.Error(w, r, http.StatusConflict, "You can't revoke your own admin access.")
		return
	case err != nil:
		respond.ServerError(w, r, err)
		return
	}
	if revokedUser != nil {
		if err := s.RevokeUserSessions(r.Context(), *revokedUser); err != nil {
			respond.ServerError(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) setStatus(status users.Status) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := r.PathValue("id")
		if !validUUID(target) {
			respond.Error(w, r, http.StatusNotFound, "We couldn't find that user.")
			return
		}
		err := s.deps.Users.SetStatus(r.Context(), target, status, UserID(r.Context()))
		switch {
		case errors.Is(err, users.ErrNotFound):
			respond.Error(w, r, http.StatusNotFound, "We couldn't find that user.")
			return
		case errors.Is(err, users.ErrSelfAction):
			respond.Error(w, r, http.StatusConflict, "You can't change your own account status.")
			return
		case err != nil:
			respond.ServerError(w, r, err)
			return
		}
		if status == users.StatusSuspended {
			if err := s.RevokeUserSessions(r.Context(), target); err != nil {
				respond.ServerError(w, r, err)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) decodeError(w http.ResponseWriter, r *http.Request, err error) {
	var reqErr *request.Error
	if errors.As(err, &reqErr) {
		respond.Error(w, r, reqErr.Status, reqErr.Message)
		return
	}
	respond.ServerError(w, r, err)
}

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
	}
	return true
}
