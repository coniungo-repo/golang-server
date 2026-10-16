// Package users owns app users, their sign-in identities, roles, admin invites and the admin audit log.
package users

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Role string

const (
	RoleCustomer Role = "customer"
	RoleProvider Role = "provider"
	RoleAdmin    Role = "admin"
)

type Kind string

const (
	KindPhone Kind = "phone"
	KindEmail Kind = "email"
	KindOIDC  Kind = "oidc"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
)

var (
	ErrSuspended     = errors.New("users: account suspended")
	ErrNotInvited    = errors.New("users: not an invited admin")
	ErrNotFound      = errors.New("users: not found")
	ErrInviteExists  = errors.New("users: an open invite already exists for this email")
	ErrInvalidEmail  = errors.New("users: invite email must be a valid address on the admin domain")
	ErrSelfAction    = errors.New("users: admins cannot do this to themselves")
	ErrIdentityTaken = errors.New("users: identity belongs to another user")
)

const uniqueViolation = "23505"

// Identity is one verified way of signing in. Subject is the E.164 phone, lowercased email, or issuer|sub.
type Identity struct {
	Kind              Kind
	Subject           string
	Email             string
	SupertokensUserID string
}

type Principal struct {
	UserID string
	Roles  []Role
}

func (p Principal) Has(r Role) bool { return slices.Contains(p.Roles, r) }

type Invite struct {
	ID         int64      `json:"id"`
	Email      string     `json:"email"`
	InvitedBy  *string    `json:"invited_by,omitempty"`
	InvitedAt  time.Time  `json:"invited_at"`
	AcceptedBy *string    `json:"accepted_by,omitempty"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

type Service struct {
	db          *pgxpool.Pool
	clock       clock.Clock
	adminDomain string
}

// New returns a Service. adminDomain is the only email domain admin invites may use, e.g. coniungolabs.com.
func New(db *pgxpool.Pool, c clock.Clock, adminDomain string) *Service {
	return &Service{db: db, clock: c, adminDomain: strings.ToLower(adminDomain)}
}

// ResolveCustomer returns the user for a phone or email sign-in, creating a customer on first sign-in.
func (s *Service) ResolveCustomer(ctx context.Context, id Identity) (Principal, error) {
	p, err := s.resolveCustomer(ctx, id)
	if isUniqueViolation(err) {
		return s.resolveCustomer(ctx, id)
	}
	return p, err
}

func (s *Service) resolveCustomer(ctx context.Context, id Identity) (Principal, error) {
	var p Principal
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetIdentityBySupertokensID(ctx, id.SupertokensUserID)
		switch {
		case err == nil:
			if Status(row.Status) != StatusActive {
				return ErrSuspended
			}
			p.UserID = row.UserID
		case errors.Is(err, pgx.ErrNoRows):
			if p.UserID, err = q.CreateUser(ctx); err != nil {
				return err
			}
			if err := s.createIdentity(ctx, q, p.UserID, id); err != nil {
				return err
			}
			if err := q.AddRole(ctx, store.AddRoleParams{UserID: p.UserID, Role: string(RoleCustomer)}); err != nil {
				return err
			}
		default:
			return err
		}
		p.Roles, err = roles(ctx, q, p.UserID)
		return err
	})
	return p, err
}

// AdminEligible reports, without changing anything, whether this OpenID Connect identity may sign in as an admin.
func (s *Service) AdminEligible(ctx context.Context, subject, addr string) error {
	q := store.New(s.db)
	row, err := q.GetIdentityBySubject(ctx, store.GetIdentityBySubjectParams{Kind: string(KindOIDC), Subject: subject})
	if err == nil {
		return s.checkActiveAdmin(ctx, q, row.UserID, Status(row.Status))
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		_, err := s.openInvite(ctx, store.New(tx), addr)
		return err
	})
}

// ResolveAdmin links an OpenID Connect identity to its admin user. The first sign-in consumes the invite for its email; later sign-ins match on subject only.
func (s *Service) ResolveAdmin(ctx context.Context, id Identity) (Principal, error) {
	var p Principal
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetIdentityBySubject(ctx, store.GetIdentityBySubjectParams{Kind: string(KindOIDC), Subject: id.Subject})
		switch {
		case err == nil:
			if row.SupertokensUserID != id.SupertokensUserID {
				return ErrIdentityTaken
			}
			if err := s.checkActiveAdmin(ctx, q, row.UserID, Status(row.Status)); err != nil {
				return err
			}
			if err := q.UpdateIdentityEmail(ctx, store.UpdateIdentityEmailParams{ID: row.ID, Email: &id.Email}); err != nil {
				return err
			}
			p.UserID = row.UserID
		case errors.Is(err, pgx.ErrNoRows):
			inviteID, err := s.openInvite(ctx, q, id.Email)
			if err != nil {
				return err
			}
			if p.UserID, err = q.CreateUser(ctx); err != nil {
				return err
			}
			if err := s.createIdentity(ctx, q, p.UserID, id); err != nil {
				return err
			}
			if err := q.AddRole(ctx, store.AddRoleParams{UserID: p.UserID, Role: string(RoleAdmin)}); err != nil {
				return err
			}
			if err := q.AcceptAdminInvite(ctx, store.AcceptAdminInviteParams{ID: inviteID, AcceptedBy: &p.UserID}); err != nil {
				return err
			}
			if err := audit(ctx, q, &p.UserID, "admin.invite.accepted", id.Email); err != nil {
				return err
			}
		default:
			return err
		}
		p.Roles, err = roles(ctx, q, p.UserID)
		return err
	})
	if isUniqueViolation(err) {
		return Principal{}, ErrIdentityTaken
	}
	return p, err
}

// IsActive reports whether userID exists and is not suspended. RequireSession calls it on every request, so suspension applies at once.
func (s *Service) IsActive(ctx context.Context, userID string) (bool, error) {
	status, err := store.New(s.db).GetUserStatus(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return Status(status) == StatusActive, err
}

// IsActiveAdmin is the authoritative per-request admin check.
func (s *Service) IsActiveAdmin(ctx context.Context, userID string) (bool, error) {
	q := store.New(s.db)
	status, err := q.GetUserStatus(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rs, err := roles(ctx, q, userID)
	if err != nil {
		return false, err
	}
	return Status(status) == StatusActive && slices.Contains(rs, RoleAdmin), nil
}

// SupertokensUserIDs lists the SuperTokens users behind userID, for revoking every session.
func (s *Service) SupertokensUserIDs(ctx context.Context, userID string) ([]string, error) {
	return store.New(s.db).ListSupertokensUserIDs(ctx, userID)
}

// NormalizeAdminEmail lowercases addr and checks it is on the admin domain.
func (s *Service) NormalizeAdminEmail(addr string) (string, error) {
	a, err := email.Normalize(addr)
	if err != nil || s.adminDomain == "" || !strings.HasSuffix(a, "@"+s.adminDomain) {
		return "", ErrInvalidEmail
	}
	return a, nil
}

func (s *Service) CreateAdminInvite(ctx context.Context, addr string, by *string) (Invite, error) {
	a, err := s.NormalizeAdminEmail(addr)
	if err != nil {
		return Invite{}, err
	}
	var inv Invite
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.CreateAdminInvite(ctx, store.CreateAdminInviteParams{Email: a, InvitedBy: by})
		if err != nil {
			return err
		}
		inv = Invite{ID: row.ID, Email: row.Email, InvitedBy: by, InvitedAt: row.InvitedAt}
		return audit(ctx, q, by, "admin.invite.created", a)
	})
	if isUniqueViolation(err) {
		return Invite{}, ErrInviteExists
	}
	return inv, err
}

func (s *Service) ListAdminInvites(ctx context.Context) ([]Invite, error) {
	rows, err := store.New(s.db).ListAdminInvites(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Invite, len(rows))
	for i, r := range rows {
		out[i] = Invite(r)
	}
	return out, nil
}

// RevokeAdminInvite closes the invite and, if it was accepted, removes that user's admin role. It returns that user's ID so the caller can revoke their sessions.
func (s *Service) RevokeAdminInvite(ctx context.Context, inviteID int64, by string) (*string, error) {
	var revoked *string
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := store.New(tx)
		acceptedBy, err := q.RevokeAdminInvite(ctx, inviteID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if acceptedBy != nil {
			if *acceptedBy == by {
				return ErrSelfAction
			}
			if _, err := q.RemoveRole(ctx, store.RemoveRoleParams{UserID: *acceptedBy, Role: string(RoleAdmin)}); err != nil {
				return err
			}
			revoked = acceptedBy
		}
		return audit(ctx, q, &by, "admin.invite.revoked", fmt.Sprint(inviteID))
	})
	return revoked, err
}

// SetStatus suspends or reactivates a user.
func (s *Service) SetStatus(ctx context.Context, userID string, status Status, by string) error {
	if userID == by {
		return ErrSelfAction
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := store.New(tx)
		n, err := q.SetUserStatus(ctx, store.SetUserStatusParams{ID: userID, Status: string(status)})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return audit(ctx, q, &by, "user."+string(status), userID)
	})
}

func (s *Service) createIdentity(ctx context.Context, q *store.Queries, userID string, id Identity) error {
	var addr *string
	if id.Email != "" {
		addr = &id.Email
	}
	return q.CreateIdentity(ctx, store.CreateIdentityParams{
		UserID:            userID,
		Kind:              string(id.Kind),
		Subject:           id.Subject,
		Email:             addr,
		SupertokensUserID: id.SupertokensUserID,
		VerifiedAt:        s.clock.Now(),
	})
}

func (s *Service) checkActiveAdmin(ctx context.Context, q *store.Queries, userID string, status Status) error {
	if status != StatusActive {
		return ErrSuspended
	}
	rs, err := roles(ctx, q, userID)
	if err != nil {
		return err
	}
	if !slices.Contains(rs, RoleAdmin) {
		return ErrNotInvited
	}
	return nil
}

func (s *Service) openInvite(ctx context.Context, q *store.Queries, addr string) (int64, error) {
	a, err := s.NormalizeAdminEmail(addr)
	if err != nil {
		return 0, ErrNotInvited
	}
	inv, err := q.GetOpenAdminInviteForUpdate(ctx, a)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && inv.AcceptedBy != nil) {
		return 0, ErrNotInvited
	}
	return inv.ID, err
}

func roles(ctx context.Context, q *store.Queries, userID string) ([]Role, error) {
	rs, err := q.ListRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Role, len(rs))
	for i, r := range rs {
		out[i] = Role(r)
	}
	return out, nil
}

func audit(ctx context.Context, q *store.Queries, actor *string, action, target string) error {
	return q.InsertAdminAudit(ctx, store.InsertAdminAuditParams{ActorID: actor, Action: action, Target: target})
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}
