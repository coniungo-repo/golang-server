package auth

import (
	"github.com/supertokens/supertokens-golang/recipe/session/sessmodels"
	"github.com/supertokens/supertokens-golang/supertokens"
)

// sessionConfig keeps SuperTokens defaults (cookies for browsers, headers for apps that send st-auth-mode: header, Secure on https, SameSite from the domains) and refuses any session without a resolved app user, which it records in the access token.
func (s *Service) sessionConfig() *sessmodels.TypeInput {
	return &sessmodels.TypeInput{
		Override: &sessmodels.OverrideStruct{
			Functions: func(original sessmodels.RecipeInterface) sessmodels.RecipeInterface {
				create := *original.CreateNewSession
				f := func(userID string, payload map[string]interface{}, data map[string]interface{}, disableAntiCsrf *bool, tenantID string, uc supertokens.UserContext) (sessmodels.SessionContainer, error) {
					in, ok := getSignIn(uc)
					if !ok {
						return nil, errNoSignIn
					}
					if payload == nil {
						payload = map[string]interface{}{}
					}
					for k, v := range sessionClaims(in, s.deps.Clock.Now()) {
						payload[k] = v
					}
					return create(userID, payload, data, disableAntiCsrf, tenantID, uc)
				}
				original.CreateNewSession = &f
				return original
			},
		},
	}
}
