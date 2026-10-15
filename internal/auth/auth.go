// Package auth wires SuperTokens passwordless sign-in (email or SMS code) and sessions into the API.
package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"

	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/phone"
	"github.com/coniungo-repo/golang-server/internal/platform/sms"
	"github.com/supertokens/supertokens-golang/recipe/passwordless"
	"github.com/supertokens/supertokens-golang/recipe/session"
	"github.com/supertokens/supertokens-golang/supertokens"
)

type Deps struct {
	Config         config.Auth
	ClientIPHeader string
	Log            *slog.Logger
	Phones         *phone.Validator
	Email          email.Sender
	SMS            sms.Sender
	Limiter        *Limiter
}

// Service is the initialised auth layer. SuperTokens keeps global state, so create one per process.
type Service struct {
	deps Deps
}

func New(d Deps) (*Service, error) {
	if d.Phones == nil || d.Email == nil || d.SMS == nil || d.Limiter == nil || d.Log == nil {
		return nil, errors.New("auth: Phones, Email, SMS, Limiter and Log are required")
	}
	s := &Service{deps: d}

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
		RecipeList: []supertokens.Recipe{
			passwordless.Init(s.passwordlessConfig()),
			session.Init(s.sessionConfig()),
		},
		Telemetry:             &telemetry,
		OnSuperTokensAPIError: s.onAPIError,
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Middleware serves the SuperTokens routes under the API base path and passes everything else to next.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return supertokens.Middleware(next)
}

// CORSHeaders lists the request headers SuperTokens frontends send.
func (s *Service) CORSHeaders() []string {
	return supertokens.GetAllCORSHeaders()
}

// RequireSession rejects requests without a valid session with 401 and passes the rest to next.
func (s *Service) RequireSession(next http.Handler) http.Handler {
	return session.VerifySession(nil, next.ServeHTTP)
}

// UserID returns the signed-in user's SuperTokens ID, or "" outside RequireSession.
func UserID(ctx context.Context) string {
	sc := session.GetSessionFromRequestContext(ctx)
	if sc == nil {
		return ""
	}
	return sc.GetUserID()
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

func (s *Service) onAPIError(err error, r *http.Request, w http.ResponseWriter) {
	s.deps.Log.ErrorContext(r.Context(), "auth request failed", "err", err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(`{"message":"Something went wrong on our side. Try again in a moment."}`))
}
