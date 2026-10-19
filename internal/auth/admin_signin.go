package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coniungo-repo/golang-server/internal/users"
	"github.com/supertokens/supertokens-golang/recipe/thirdparty/tpmodels"
	"github.com/supertokens/supertokens-golang/supertokens"
)

const (
	AdminProviderID    = "staff"
	adminDeniedMessage = "This account can't sign in here. Ask an admin for an invite."
	wellKnownSuffix    = "/.well-known/openid-configuration"
)

var errAdminClaims = errors.New("auth: admin ID token rejected")

func (s *Service) thirdPartyConfig() *tpmodels.TypeInput {
	a := s.deps.Admin
	pkce := true
	requireEmail := true
	params := map[string]interface{}{"prompt": "select_account"}
	if a.HostedDomain != "" {
		params["hd"] = a.HostedDomain
	}
	return &tpmodels.TypeInput{
		SignInAndUpFeature: tpmodels.TypeInputSignInAndUp{
			Providers: []tpmodels.ProviderInput{{
				Config: tpmodels.ProviderConfig{
					ThirdPartyId:                     AdminProviderID,
					Name:                             "Staff",
					OIDCDiscoveryEndpoint:            strings.TrimSuffix(a.Issuer, "/") + wellKnownSuffix,
					AuthorizationEndpointQueryParams: params,
					RequireEmail:                     &requireEmail,
					ValidateIdTokenPayload: func(payload map[string]interface{}, _ tpmodels.ProviderConfigForClientType, _ supertokens.UserContext) error {
						return s.validateAdminClaims(payload)
					},
					Clients: []tpmodels.ProviderClientConfig{{
						ClientID:     a.ClientID,
						ClientSecret: a.ClientSecret.Reveal(),
						Scope:        []string{"openid", "email"},
						ForcePKCE:    &pkce,
					}},
				},
			}},
		},
		Override: &tpmodels.OverrideStruct{
			Functions: s.overrideThirdPartyFunctions,
			APIs:      s.overrideThirdPartyAPIs,
		},
	}
}

// validateAdminClaims checks what the SDK does not: issuer, audience, verified email, admin domain and, for Google Workspace, the hosted domain.
func (s *Service) validateAdminClaims(c map[string]interface{}) error {
	a := s.deps.Admin
	reject := func(reason string) error { return fmt.Errorf("%w: %s", errAdminClaims, reason) }

	if len(c) == 0 {
		return reject("no verified ID token")
	}
	iss, _ := c["iss"].(string)
	issuer := strings.TrimSuffix(a.Issuer, "/")
	if iss != issuer && iss != strings.TrimPrefix(issuer, "https://") {
		return reject("issuer mismatch")
	}
	if !audienceContains(c["aud"], c["azp"], a.ClientID) {
		return reject("audience mismatch")
	}
	if sub, _ := c["sub"].(string); sub == "" {
		return reject("missing subject")
	}
	if !truthy(c["email_verified"]) {
		return reject("email not verified")
	}
	addr, _ := c["email"].(string)
	if !strings.HasSuffix(strings.ToLower(addr), "@"+a.EmailDomain) {
		return reject("email outside the admin domain")
	}
	if a.HostedDomain != "" {
		if hd, _ := c["hd"].(string); !strings.EqualFold(hd, a.HostedDomain) {
			return reject("hosted domain mismatch")
		}
	}
	return nil
}

func (s *Service) adminSubject(sub string) string {
	return strings.TrimSuffix(s.deps.Admin.Issuer, "/") + "|" + sub
}

func (s *Service) overrideThirdPartyFunctions(original tpmodels.RecipeInterface) tpmodels.RecipeInterface {
	signInUp := *original.SignInUp

	f := func(thirdPartyID string, thirdPartyUserID string, emailAddr string, oAuthTokens tpmodels.TypeOAuthTokens, raw tpmodels.TypeRawUserInfoFromProvider, tenantID string, uc supertokens.UserContext) (tpmodels.SignInUpResponse, error) {
		claims := raw.FromIdTokenPayload
		if err := s.validateAdminClaims(claims); err != nil {
			return tpmodels.SignInUpResponse{}, err
		}
		sub, _ := claims["sub"].(string)
		addr := strings.ToLower(claims["email"].(string))
		subject := s.adminSubject(sub)
		ctx := requestContext(uc)

		if err := s.deps.Users.AdminEligible(ctx, subject, addr); err != nil {
			return tpmodels.SignInUpResponse{}, err
		}
		resp, err := signInUp(thirdPartyID, thirdPartyUserID, addr, oAuthTokens, raw, tenantID, uc)
		if err != nil || resp.OK == nil {
			return resp, err
		}
		p, err := s.deps.Users.ResolveAdmin(ctx, users.Identity{
			Kind:              users.KindOIDC,
			Subject:           subject,
			Email:             addr,
			SupertokensUserID: resp.OK.User.ID,
		})
		if err != nil {
			return tpmodels.SignInUpResponse{}, err
		}
		putSignIn(uc, signIn{Principal: p, Method: MethodOIDC})
		return resp, nil
	}

	original.SignInUp = &f
	return original
}

func (s *Service) overrideThirdPartyAPIs(original tpmodels.APIInterface) tpmodels.APIInterface {
	authURL := *original.AuthorisationUrlGET
	signInUp := *original.SignInUpPOST

	u := func(provider *tpmodels.TypeProvider, redirectURI string, tenantID string, options tpmodels.APIOptions, uc supertokens.UserContext) (tpmodels.AuthorisationUrlGETResponse, error) {
		if !s.onAdminHost(options.Req) {
			return tpmodels.AuthorisationUrlGETResponse{GeneralError: &supertokens.GeneralErrorResponse{Message: adminDeniedMessage}}, nil
		}
		return authURL(provider, redirectURI, tenantID, options, uc)
	}

	p := func(provider *tpmodels.TypeProvider, input tpmodels.TypeSignInUpInput, tenantID string, options tpmodels.APIOptions, uc supertokens.UserContext) (tpmodels.SignInUpPOSTResponse, error) {
		denied := tpmodels.SignInUpPOSTResponse{GeneralError: &supertokens.GeneralErrorResponse{Message: adminDeniedMessage}}
		if !s.onAdminHost(options.Req) || input.RedirectURIInfo == nil {
			return denied, nil
		}
		resp, err := signInUp(provider, input, tenantID, options, uc)
		if isAdminDenial(err) {
			s.deps.Log.WarnContext(options.Req.Context(), "admin sign-in refused", "reason", err.Error())
			return denied, nil
		}
		return resp, err
	}

	original.AuthorisationUrlGET = &u
	original.SignInUpPOST = &p
	return original
}

func isAdminDenial(err error) bool {
	return errors.Is(err, errAdminClaims) ||
		errors.Is(err, users.ErrNotInvited) ||
		errors.Is(err, users.ErrSuspended) ||
		errors.Is(err, users.ErrIdentityTaken)
}

func audienceContains(aud, azp interface{}, clientID string) bool {
	switch v := aud.(type) {
	case string:
		return v == clientID
	case []interface{}:
		found := false
		for _, a := range v {
			if a == clientID {
				found = true
			}
		}
		if len(v) > 1 {
			p, _ := azp.(string)
			return found && p == clientID
		}
		return found
	}
	return false
}

func truthy(v interface{}) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "true"
	}
	return false
}

func requestContext(uc supertokens.UserContext) context.Context {
	if r := supertokens.GetRequestFromUserContext(uc); r != nil {
		return r.Context()
	}
	return context.Background()
}
