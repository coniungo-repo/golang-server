package auth

import (
	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/phone"
	"github.com/coniungo-repo/golang-server/internal/users"
	"github.com/supertokens/supertokens-golang/ingredients/emaildelivery"
	"github.com/supertokens/supertokens-golang/ingredients/smsdelivery"
	"github.com/supertokens/supertokens-golang/recipe/passwordless/plessmodels"
	"github.com/supertokens/supertokens-golang/supertokens"
)

const (
	flowUserInputCode   = "USER_INPUT_CODE"
	invalidEmailMessage = "Enter a valid email address."
)

func (s *Service) passwordlessConfig() plessmodels.TypeInput {
	sendEmail := s.sendEmail
	sendSMS := s.sendSMS

	return plessmodels.TypeInput{
		FlowType: flowUserInputCode,
		ContactMethodEmailOrPhone: plessmodels.ContactMethodEmailOrPhoneConfig{
			Enabled:              true,
			ValidateEmailAddress: s.validateEmail,
			ValidatePhoneNumber:  s.validatePhone,
		},
		EmailDelivery: &emaildelivery.TypeInput{
			Service: &emaildelivery.EmailDeliveryInterface{SendEmail: &sendEmail},
		},
		SmsDelivery: &smsdelivery.TypeInput{
			Service: &smsdelivery.SmsDeliveryInterface{SendSms: &sendSMS},
		},
		Override: &plessmodels.OverrideStruct{
			Functions: s.overridePasswordlessFunctions,
			APIs:      s.overrideAPIs,
		},
	}
}

func (s *Service) validateEmail(value interface{}, _ string) *string {
	raw, _ := value.(string)
	if _, err := email.Normalize(raw); err != nil {
		msg := invalidEmailMessage
		return &msg
	}
	return nil
}

func (s *Service) validatePhone(value interface{}, _ string) *string {
	raw, _ := value.(string)
	if _, err := s.deps.Phones.Parse(raw); err != nil {
		msg := phone.Message(err)
		return &msg
	}
	return nil
}

const suspendedMessage = "This account is suspended. Contact support."

// overridePasswordlessFunctions links each successful code sign-in to an app user, creating a customer on first sign-in.
func (s *Service) overridePasswordlessFunctions(original plessmodels.RecipeInterface) plessmodels.RecipeInterface {
	consume := *original.ConsumeCode

	f := func(userInput *plessmodels.UserInputCodeWithDeviceID, linkCode *string, preAuthSessionID string, tenantID string, uc supertokens.UserContext) (plessmodels.ConsumeCodeResponse, error) {
		resp, err := consume(userInput, linkCode, preAuthSessionID, tenantID, uc)
		if err != nil || resp.OK == nil {
			return resp, err
		}
		id, method, err := passwordlessIdentity(resp.OK.User)
		if err != nil {
			return plessmodels.ConsumeCodeResponse{}, err
		}
		p, err := s.deps.Users.ResolveCustomer(requestContext(uc), id)
		if err != nil {
			return plessmodels.ConsumeCodeResponse{}, err
		}
		putSignIn(uc, signIn{Principal: p, Method: method})
		return resp, nil
	}

	original.ConsumeCode = &f
	return original
}

func passwordlessIdentity(u plessmodels.User) (users.Identity, string, error) {
	switch {
	case u.PhoneNumber != nil:
		return users.Identity{Kind: users.KindPhone, Subject: *u.PhoneNumber, SupertokensUserID: u.ID}, MethodPhoneOTP, nil
	case u.Email != nil:
		return users.Identity{Kind: users.KindEmail, Subject: *u.Email, Email: *u.Email, SupertokensUserID: u.ID}, MethodEmailOTP, nil
	default:
		return users.Identity{}, "", errNoContact
	}
}
