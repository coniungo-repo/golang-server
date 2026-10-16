package auth

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/coniungo-repo/golang-server/internal/users"
	"github.com/supertokens/supertokens-golang/recipe/session"
	"github.com/supertokens/supertokens-golang/supertokens"
)

const (
	claimUserID   = "uid"
	claimRoles    = "roles"
	claimMethod   = "amr"
	claimAuthTime = "auth_time"

	MethodPhoneOTP = "phone_otp"
	MethodEmailOTP = "email_otp"
	MethodOIDC     = "oidc"

	signInContextKey = "coniungo.sign_in"
)

var errNoSignIn = errors.New("auth: session requested without a resolved app user")

// Principal is the signed-in user as recorded in the session at sign-in.
type Principal struct {
	UserID   string
	Roles    []users.Role
	Method   string
	AuthTime time.Time
}

func (p Principal) Has(r users.Role) bool { return slices.Contains(p.Roles, r) }

type signIn struct {
	users.Principal
	Method string
}

type principalKey struct{}

// FromContext returns the principal placed on the request by RequireSession.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// UserID returns the signed-in app user's ID, or "" outside RequireSession.
func UserID(ctx context.Context) string {
	p, _ := FromContext(ctx)
	return p.UserID
}

func putSignIn(uc supertokens.UserContext, s signIn) {
	if uc == nil {
		return
	}
	if *uc == nil {
		*uc = map[string]interface{}{}
	}
	(*uc)[signInContextKey] = s
}

func getSignIn(uc supertokens.UserContext) (signIn, bool) {
	if uc == nil || *uc == nil {
		return signIn{}, false
	}
	s, ok := (*uc)[signInContextKey].(signIn)
	return s, ok
}

func sessionClaims(s signIn, now time.Time) map[string]interface{} {
	roles := make([]string, len(s.Roles))
	for i, r := range s.Roles {
		roles[i] = string(r)
	}
	return map[string]interface{}{
		claimUserID:   s.UserID,
		claimRoles:    roles,
		claimMethod:   s.Method,
		claimAuthTime: now.Unix(),
	}
}

func principalFromClaims(payload map[string]interface{}) (Principal, bool) {
	uid, _ := payload[claimUserID].(string)
	method, _ := payload[claimMethod].(string)
	authTime, ok := payload[claimAuthTime].(float64)
	if uid == "" || method == "" || !ok {
		return Principal{}, false
	}
	p := Principal{UserID: uid, Method: method, AuthTime: time.Unix(int64(authTime), 0)}
	raw, _ := payload[claimRoles].([]interface{})
	for _, r := range raw {
		if s, ok := r.(string); ok {
			p.Roles = append(p.Roles, users.Role(s))
		}
	}
	return p, true
}

func principalFromSession(ctx context.Context) (Principal, string, bool) {
	sc := session.GetSessionFromRequestContext(ctx)
	if sc == nil {
		return Principal{}, "", false
	}
	p, ok := principalFromClaims(sc.GetAccessTokenPayload())
	return p, sc.GetUserID(), ok
}
