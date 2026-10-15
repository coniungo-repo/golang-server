package auth

import (
	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/phone"
	"github.com/supertokens/supertokens-golang/ingredients/emaildelivery"
	"github.com/supertokens/supertokens-golang/ingredients/smsdelivery"
	"github.com/supertokens/supertokens-golang/recipe/passwordless/plessmodels"
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
			APIs: s.overrideAPIs,
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
