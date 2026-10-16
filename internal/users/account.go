package users

import (
	"context"
	"errors"
	"time"

	"github.com/coniungo-repo/golang-server/internal/store"
	"github.com/jackc/pgx/v5"
)

var ErrLastIdentity = errors.New("users: cannot remove the only way to sign in")

// LinkedIdentity is an identity as shown to its owner. Value is the phone or email, or the staff email for oidc.
type LinkedIdentity struct {
	ID         int64     `json:"id"`
	Kind       Kind      `json:"kind"`
	Value      string    `json:"value"`
	VerifiedAt time.Time `json:"verified_at"`
}

func (s *Service) ListIdentities(ctx context.Context, userID string) ([]LinkedIdentity, error) {
	rows, err := store.New(s.db).ListIdentities(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]LinkedIdentity, len(rows))
	for i, r := range rows {
		value := r.Subject
		if Kind(r.Kind) == KindOIDC && r.Email != nil {
			value = *r.Email
		}
		out[i] = LinkedIdentity{ID: r.ID, Kind: Kind(r.Kind), Value: value, VerifiedAt: r.VerifiedAt}
	}
	return out, nil
}

// HasIdentity reports whether kind and subject are already linked to userID. It says nothing about other users.
func (s *Service) HasIdentity(ctx context.Context, userID string, kind Kind, subject string) (bool, error) {
	row, err := store.New(s.db).GetIdentityBySubject(ctx, store.GetIdentityBySubjectParams{Kind: string(kind), Subject: subject})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil && row.UserID == userID, err
}

// LinkIdentity adds a verified identity to an existing active user. Linking one the user already has is a no-op; one held by anyone else is ErrIdentityTaken.
func (s *Service) LinkIdentity(ctx context.Context, userID string, id Identity) error {
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := lockActive(ctx, q, userID); err != nil {
			return err
		}
		existing, err := q.GetIdentityBySubject(ctx, store.GetIdentityBySubjectParams{Kind: string(id.Kind), Subject: id.Subject})
		switch {
		case err == nil:
			if existing.UserID != userID || existing.SupertokensUserID != id.SupertokensUserID {
				return ErrIdentityTaken
			}
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		return s.createIdentity(ctx, q, userID, id)
	})
	if isUniqueViolation(err) {
		return ErrIdentityTaken
	}
	return err
}

// RemoveIdentity unlinks one of userID's identities, refusing the last one. release runs inside the transaction, before the row is deleted, with that identity's SuperTokens user ID; if it fails nothing is removed.
func (s *Service) RemoveIdentity(ctx context.Context, userID string, identityID int64, release func(supertokensUserID string) error) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := lockActive(ctx, q, userID); err != nil {
			return err
		}
		stID, err := q.GetIdentityForUser(ctx, store.GetIdentityForUserParams{ID: identityID, UserID: userID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		n, err := q.CountIdentities(ctx, userID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrLastIdentity
		}
		if err := release(stID); err != nil {
			return err
		}
		return q.DeleteIdentity(ctx, identityID)
	})
}

// DeleteUser deletes userID and, by cascade, their identities and roles. release runs inside the transaction, before the delete, with every SuperTokens user ID they sign in with; if it fails nothing is deleted. A non-nil by records an admin deletion in the audit log.
func (s *Service) DeleteUser(ctx context.Context, userID string, by *string, release func(supertokensUserIDs []string) error) error {
	if by != nil && *by == userID {
		return ErrSelfAction
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := store.New(tx)
		_, err := q.LockUser(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		ids, err := q.ListSupertokensUserIDs(ctx, userID)
		if err != nil {
			return err
		}
		if err := release(ids); err != nil {
			return err
		}
		if by != nil {
			if err := audit(ctx, q, by, "user.deleted", userID); err != nil {
				return err
			}
		}
		return q.DeleteUser(ctx, userID)
	})
}

func lockActive(ctx context.Context, q *store.Queries, userID string) error {
	status, err := q.LockUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if Status(status) != StatusActive {
		return ErrSuspended
	}
	return nil
}
