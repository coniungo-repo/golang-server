package auth

import "github.com/supertokens/supertokens-golang/recipe/session/sessmodels"

// sessionConfig keeps SuperTokens defaults: cookies for browsers, headers for apps that send st-auth-mode: header, Secure when the API domain is https, and SameSite chosen from the API and website domains.
func (s *Service) sessionConfig() *sessmodels.TypeInput {
	return &sessmodels.TypeInput{}
}
