package auth

import (
	"errors"
	"net/http"

	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/request"
	"github.com/coniungo-repo/golang-server/internal/users"
	"github.com/supertokens/supertokens-golang/recipe/passwordless/plessmodels"
	"github.com/supertokens/supertokens-golang/supertokens"
)

const tooManyCodesMessage = "Too many codes requested. Wait a few minutes and try again."

var errNoContact = errors.New("auth: request has neither email nor phone number")

// overrideAPIs canonicalises contacts before codes are created, rate limits code sends, and disables the exists endpoints that would let anyone enumerate accounts.
func (s *Service) overrideAPIs(original plessmodels.APIInterface) plessmodels.APIInterface {
	createCode := *original.CreateCodePOST
	resendCode := *original.ResendCodePOST
	consumeCode := *original.ConsumeCodePOST

	consume := func(userInput *plessmodels.UserInputCodeWithDeviceID, linkCode *string, preAuthSessionID string, tenantID string, options plessmodels.APIOptions, userContext supertokens.UserContext) (plessmodels.ConsumeCodePOSTResponse, error) {
		resp, err := consumeCode(userInput, linkCode, preAuthSessionID, tenantID, options, userContext)
		if errors.Is(err, users.ErrSuspended) {
			return plessmodels.ConsumeCodePOSTResponse{GeneralError: &supertokens.GeneralErrorResponse{Message: suspendedMessage}}, nil
		}
		return resp, err
	}

	create := func(emailAddr *string, phoneNumber *string, tenantID string, options plessmodels.APIOptions, userContext supertokens.UserContext) (plessmodels.CreateCodePOSTResponse, error) {
		destination, err := s.canonicalise(emailAddr, phoneNumber)
		if err != nil {
			return plessmodels.CreateCodePOSTResponse{}, err
		}
		allowed, err := s.allowSend(options.Req, destination)
		if err != nil {
			return plessmodels.CreateCodePOSTResponse{}, err
		}
		if !allowed {
			return plessmodels.CreateCodePOSTResponse{GeneralError: &supertokens.GeneralErrorResponse{Message: tooManyCodesMessage}}, nil
		}
		return createCode(emailAddr, phoneNumber, tenantID, options, userContext)
	}

	resend := func(deviceID string, preAuthSessionID string, tenantID string, options plessmodels.APIOptions, userContext supertokens.UserContext) (plessmodels.ResendCodePOSTResponse, error) {
		device, err := (*options.RecipeImplementation.ListCodesByDeviceID)(deviceID, tenantID, userContext)
		if err != nil {
			return plessmodels.ResendCodePOSTResponse{}, err
		}
		if device == nil || device.PreAuthSessionID != preAuthSessionID {
			return resendCode(deviceID, preAuthSessionID, tenantID, options, userContext)
		}
		destination, err := contactKey(device.Email, device.PhoneNumber)
		if err != nil {
			return plessmodels.ResendCodePOSTResponse{}, err
		}
		allowed, err := s.allowSend(options.Req, destination)
		if err != nil {
			return plessmodels.ResendCodePOSTResponse{}, err
		}
		if !allowed {
			return plessmodels.ResendCodePOSTResponse{GeneralError: &supertokens.GeneralErrorResponse{Message: tooManyCodesMessage}}, nil
		}
		return resendCode(deviceID, preAuthSessionID, tenantID, options, userContext)
	}

	original.ConsumeCodePOST = &consume
	original.CreateCodePOST = &create
	original.ResendCodePOST = &resend
	original.EmailExistsGET = nil
	original.PhoneNumberExistsGET = nil
	return original
}

// canonicalise rewrites the contact in place to its stored form, so one person maps to one account however they type it.
func (s *Service) canonicalise(emailAddr, phoneNumber *string) (string, error) {
	switch {
	case emailAddr != nil:
		normalized, err := email.Normalize(*emailAddr)
		if err != nil {
			return "", err
		}
		*emailAddr = normalized
	case phoneNumber != nil:
		n, err := s.deps.Phones.Parse(*phoneNumber)
		if err != nil {
			return "", err
		}
		*phoneNumber = n.E164()
	}
	return contactKey(emailAddr, phoneNumber)
}

func contactKey(emailAddr, phoneNumber *string) (string, error) {
	switch {
	case emailAddr != nil:
		return "email:" + *emailAddr, nil
	case phoneNumber != nil:
		return "phone:" + *phoneNumber, nil
	default:
		return "", errNoContact
	}
}

func (s *Service) allowSend(r *http.Request, destination string) (bool, error) {
	return s.deps.Limiter.Allow(r.Context(),
		Attempt{Policy: s.clientPolicy, Value: request.ClientIP(r, s.deps.ClientIPHeader)},
		Attempt{Policy: s.destinationPolicy, Value: destination},
	)
}
