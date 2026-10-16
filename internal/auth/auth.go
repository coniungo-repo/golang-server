// Package auth wires SuperTokens passwordless sign-in (email or SMS code) and sessions into the API.
package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/phone"
	"github.com/coniungo-repo/golang-server/internal/platform/requestid"
	"github.com/coniungo-repo/golang-server/internal/platform/sms"
	"github.com/coniungo-repo/golang-server/internal/users"
	"github.com/supertokens/supertokens-golang/recipe/passwordless"
	"github.com/supertokens/supertokens-golang/recipe/session"
	"github.com/supertokens/supertokens-golang/recipe/thirdparty"
	"github.com/supertokens/supertokens-golang/supertokens"
)

type Deps struct {
	Config         config.Auth
	Admin          config.Admin
	Clock          clock.Clock
	Users          *users.Service
	ClientIPHeader string
	Log            *slog.Logger
	Phones         *phone.Validator
	Email          email.Sender
	SMS            sms.Sender
	Limiter        *Limiter
}

// Service is the initialised auth layer. SuperTokens keeps global state, so create one per process.
type Service struct {
	deps              Deps
	destinationPolicy Policy
	clientPolicy      Policy
}

func New(d Deps) (*Service, error) {
	if d.Phones == nil || d.Email == nil || d.SMS == nil || d.Limiter == nil || d.Log == nil || d.Users == nil || d.Clock == nil {
		return nil, errors.New("auth: Phones, Email, SMS, Limiter, Users, Clock and Log are required")
	}
	limits := d.Config.RateLimits
	if len(limits.Destination) == 0 || len(limits.ClientIP) == 0 {
		return nil, errors.New("auth: destination and client IP rate limits are required")
	}
	s := &Service{
		deps:              d,
		destinationPolicy: Policy{Kind: kindDestination, Windows: limits.Destination},
		clientPolicy:      Policy{Kind: kindClientIP, Windows: limits.ClientIP},
	}

	recipes := []supertokens.Recipe{
		passwordless.Init(s.passwordlessConfig()),
		session.Init(s.sessionConfig()),
	}
	if d.Admin.Enabled() {
		recipes = append(recipes, thirdparty.Init(s.thirdPartyConfig()))
	}

	telemetry := false
	err := supertokens.Init(supertokens.TypeInput{
		Supertokens: &supertokens.ConnectionInfo{
			ConnectionURI: d.Config.ConnectionURI,
			APIKey:        d.Config.APIKey.Reveal(),
		},
		AppInfo: supertokens.AppInfo{
			AppName:         d.Config.AppName,
			APIDomain:       d.Config.APIDomain,
			APIBasePath:     &d.Config.APIBasePath,
			WebsiteBasePath: &d.Config.WebsiteBasePath,
			GetOrigin:       s.origin,
		},
		RecipeList:            recipes,
		Telemetry:             &telemetry,
		OnSuperTokensAPIError: s.onAPIError,
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Middleware serves the SuperTokens routes under the API base path and passes everything else to next. Error bodies on those routes carry the request ID.
func (s *Service) Middleware(next http.Handler) http.Handler {
	st := supertokens.Middleware(next)
	base := s.deps.Config.APIBasePath
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, base) {
			st.ServeHTTP(w, r)
			return
		}
		ew := &errorWriter{ResponseWriter: w}
		st.ServeHTTP(ew, r)
		ew.finish(requestid.From(r.Context()))
	})
}

// CORSHeaders lists the request headers SuperTokens frontends send.
func (s *Service) CORSHeaders() []string {
	return supertokens.GetAllCORSHeaders()
}

// RevokeUserSessions signs userID out everywhere, across every identity they sign in with.
func (s *Service) RevokeUserSessions(ctx context.Context, userID string) error {
	ids, err := s.deps.Users.SupertokensUserIDs(ctx, userID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := session.RevokeAllSessionsForUser(id, nil); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) origin(r *http.Request, _ supertokens.UserContext) (string, error) {
	domains := s.deps.Config.WebsiteDomains
	if r != nil {
		if o := r.Header.Get("Origin"); slices.Contains(domains, o) {
			return o, nil
		}
	}
	return domains[0], nil
}
