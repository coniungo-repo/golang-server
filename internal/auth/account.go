package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/coniungo-repo/golang-server/internal/platform/respond"
	"github.com/coniungo-repo/golang-server/internal/users"
	"github.com/supertokens/supertokens-golang/recipe/session"
	"github.com/supertokens/supertokens-golang/supertokens"
)

const userNotFoundMessage = "We couldn't find that user."

// DeleteAccount deletes userID from our database and every SuperTokens user they sign in with, revoking their sessions first. Pass the acting admin as by to record it in the audit log, or nil when users delete themselves. Nothing in our database is removed unless the core calls succeed, so a failed deletion can be retried.
func (s *Service) DeleteAccount(ctx context.Context, userID string, by *string) error {
	return s.deps.Users.DeleteUser(ctx, userID, by, s.releaseSupertokensUsers)
}

func (s *Service) releaseSupertokensUsers(ids []string) error {
	for _, id := range ids {
		if _, err := session.RevokeAllSessionsForUser(id, nil); err != nil {
			return err
		}
	}
	for _, id := range ids {
		if err := supertokens.DeleteUser(id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) deleteMe(w http.ResponseWriter, r *http.Request) {
	userID := UserID(r.Context())
	if err := s.DeleteAccount(r.Context(), userID, nil); err != nil {
		respond.ServerError(w, r, err)
		return
	}
	s.deps.Log.InfoContext(r.Context(), "account deleted", "user_id", userID)
	if sc := session.GetSessionFromRequestContext(r.Context()); sc != nil {
		if err := sc.RevokeSession(); err != nil {
			s.deps.Log.WarnContext(r.Context(), "clearing session tokens failed", "err", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) deleteUser(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("id")
	if !validUUID(target) {
		respond.Error(w, r, http.StatusNotFound, userNotFoundMessage)
		return
	}
	actor := UserID(r.Context())
	err := s.DeleteAccount(r.Context(), target, &actor)
	switch {
	case errors.Is(err, users.ErrNotFound):
		respond.Error(w, r, http.StatusNotFound, userNotFoundMessage)
	case errors.Is(err, users.ErrSelfAction):
		respond.Error(w, r, http.StatusConflict, "You can't delete your own account here.")
	case err != nil:
		respond.ServerError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
