package auth

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/phone"
	"github.com/coniungo-repo/golang-server/internal/platform/request"
	"github.com/coniungo-repo/golang-server/internal/platform/respond"
	"github.com/coniungo-repo/golang-server/internal/users"
	"github.com/supertokens/supertokens-golang/ingredients/emaildelivery"
	"github.com/supertokens/supertokens-golang/ingredients/smsdelivery"
	"github.com/supertokens/supertokens-golang/recipe/passwordless"
	"github.com/supertokens/supertokens-golang/recipe/passwordless/plessmodels"
	"github.com/supertokens/supertokens-golang/supertokens"
)

const (
	tenantPublic      = "public"
	linkingContextKey = "coniungo.linking"

	oneContactMessage       = "Enter an email address or a phone number."
	alreadyLinkedMessage    = "That's already on your account."
	identityTakenMessage    = "That can't be added to this account. If it's yours, sign in with it and contact support."
	staffContactsMessage    = "Staff accounts sign in with Google only."
	codeFieldsMessage       = "Enter the code we sent you."
	wrongCodeMessage        = "That code isn't right. Check it and try again."
	expiredCodeMessage      = "That code has expired. Request a new one."
	restartFlowMessage      = "Request a new code and try again."
	identityNotFoundMessage = "We couldn't find that sign-in method."
	lastIdentityMessage     = "Add another way to sign in before removing this one."
)

type contactRequest struct {
	Email       *string `json:"email"`
	PhoneNumber *string `json:"phone_number"`
}

type codeResponse struct {
	DeviceID         string `json:"device_id"`
	PreAuthSessionID string `json:"pre_auth_session_id"`
}

type verifyRequest struct {
	DeviceID         string `json:"device_id"`
	PreAuthSessionID string `json:"pre_auth_session_id"`
	Code             string `json:"code"`
}

type identitiesResponse struct {
	Identities []users.LinkedIdentity `json:"identities"`
}

func (s *Service) listIdentities(w http.ResponseWriter, r *http.Request) {
	ids, err := s.deps.Users.ListIdentities(r.Context(), UserID(r.Context()))
	if err != nil {
		respond.ServerError(w, r, err)
		return
	}
	respond.JSON(w, r, http.StatusOK, identitiesResponse{Identities: ids})
}

// startIdentity sends a code to a new email or phone for the signed-in user. Whether another account holds it is only checked after the code proves ownership, so this never reveals who is registered.
func (s *Service) startIdentity(w http.ResponseWriter, r *http.Request) {
	p, _ := FromContext(r.Context())
	if p.Has(users.RoleAdmin) {
		respond.Error(w, r, http.StatusForbidden, staffContactsMessage)
		return
	}
	var body contactRequest
	if err := request.Decode(w, r, &body); err != nil {
		s.decodeError(w, r, err)
		return
	}
	kind, subject, msg := s.parseContact(body)
	if msg != "" {
		respond.Error(w, r, http.StatusUnprocessableEntity, msg)
		return
	}
	linked, err := s.deps.Users.HasIdentity(r.Context(), p.UserID, kind, subject)
	if err != nil {
		respond.ServerError(w, r, err)
		return
	}
	if linked {
		respond.Error(w, r, http.StatusConflict, alreadyLinkedMessage)
		return
	}
	allowed, err := s.allowSend(r, string(kind)+":"+subject)
	if err != nil {
		respond.ServerError(w, r, err)
		return
	}
	if !allowed {
		respond.Error(w, r, http.StatusTooManyRequests, tooManyCodesMessage)
		return
	}

	uc := supertokens.MakeDefaultUserContextFromAPI(r)
	code, err := s.sendLinkCode(kind, subject, uc)
	if err != nil {
		respond.ServerError(w, r, err)
		return
	}
	respond.JSON(w, r, http.StatusCreated, codeResponse{DeviceID: code.DeviceID, PreAuthSessionID: code.PreAuthSessionID})
}

// verifyIdentity consumes the code and links the proven email or phone to the signed-in user.
func (s *Service) verifyIdentity(w http.ResponseWriter, r *http.Request) {
	p, _ := FromContext(r.Context())
	if p.Has(users.RoleAdmin) {
		respond.Error(w, r, http.StatusForbidden, staffContactsMessage)
		return
	}
	var body verifyRequest
	if err := request.Decode(w, r, &body); err != nil {
		s.decodeError(w, r, err)
		return
	}
	if body.DeviceID == "" || body.PreAuthSessionID == "" || body.Code == "" {
		respond.Error(w, r, http.StatusUnprocessableEntity, codeFieldsMessage)
		return
	}

	uc := supertokens.MakeDefaultUserContextFromAPI(r)
	markLinking(uc)
	resp, err := passwordless.ConsumeCodeWithUserInputCode(tenantPublic, body.DeviceID, body.Code, body.PreAuthSessionID, uc)
	switch {
	case err != nil:
		respond.ServerError(w, r, err)
		return
	case resp.IncorrectUserInputCodeError != nil:
		respond.Error(w, r, http.StatusUnprocessableEntity, wrongCodeMessage)
		return
	case resp.ExpiredUserInputCodeError != nil:
		respond.Error(w, r, http.StatusUnprocessableEntity, expiredCodeMessage)
		return
	case resp.OK == nil:
		respond.Error(w, r, http.StatusUnprocessableEntity, restartFlowMessage)
		return
	}

	id, _, err := passwordlessIdentity(resp.OK.User)
	if err != nil {
		respond.ServerError(w, r, err)
		return
	}
	err = s.deps.Users.LinkIdentity(r.Context(), p.UserID, id)
	if err != nil && resp.OK.CreatedNewUser {
		if delErr := supertokens.DeleteUser(id.SupertokensUserID); delErr != nil {
			s.deps.Log.ErrorContext(r.Context(), "removing unlinked sign-in user failed", "err", delErr)
		}
	}
	switch {
	case errors.Is(err, users.ErrIdentityTaken):
		s.deps.Log.WarnContext(r.Context(), "contact already linked to another user", "kind", id.Kind)
		respond.Error(w, r, http.StatusConflict, identityTakenMessage)
		return
	case errors.Is(err, users.ErrSuspended), errors.Is(err, users.ErrNotFound):
		respond.Error(w, r, http.StatusUnauthorized, signInAgainMessage)
		return
	case err != nil:
		respond.ServerError(w, r, err)
		return
	}
	s.deps.Log.InfoContext(r.Context(), "contact added", "kind", id.Kind)

	ids, err := s.deps.Users.ListIdentities(r.Context(), p.UserID)
	if err != nil {
		respond.ServerError(w, r, err)
		return
	}
	respond.JSON(w, r, http.StatusCreated, identitiesResponse{Identities: ids})
}

// removeIdentity unlinks one of the user's sign-in methods, signs out its sessions and deletes its SuperTokens user. The last one cannot be removed.
func (s *Service) removeIdentity(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		respond.Error(w, r, http.StatusNotFound, identityNotFoundMessage)
		return
	}
	err = s.deps.Users.RemoveIdentity(r.Context(), UserID(r.Context()), id, func(stID string) error {
		return s.releaseSupertokensUsers([]string{stID})
	})
	switch {
	case errors.Is(err, users.ErrNotFound):
		respond.Error(w, r, http.StatusNotFound, identityNotFoundMessage)
	case errors.Is(err, users.ErrLastIdentity):
		respond.Error(w, r, http.StatusConflict, lastIdentityMessage)
	case errors.Is(err, users.ErrSuspended):
		respond.Error(w, r, http.StatusUnauthorized, suspendedMessage)
	case err != nil:
		respond.ServerError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) parseContact(body contactRequest) (users.Kind, string, string) {
	switch {
	case body.Email != nil && body.PhoneNumber == nil:
		addr, err := email.Normalize(*body.Email)
		if err != nil {
			return "", "", invalidEmailMessage
		}
		return users.KindEmail, addr, ""
	case body.PhoneNumber != nil && body.Email == nil:
		n, err := s.deps.Phones.Parse(*body.PhoneNumber)
		if err != nil {
			return "", "", phone.Message(err)
		}
		return users.KindPhone, n.E164(), ""
	default:
		return "", "", oneContactMessage
	}
}

// sendLinkCode creates a code in the core and delivers it with the same templates as sign-in.
func (s *Service) sendLinkCode(kind users.Kind, subject string, uc supertokens.UserContext) (*plessmodels.NewCode, error) {
	var (
		resp plessmodels.CreateCodeResponse
		err  error
	)
	if kind == users.KindEmail {
		resp, err = passwordless.CreateCodeWithEmail(tenantPublic, subject, nil, uc)
	} else {
		resp, err = passwordless.CreateCodeWithPhoneNumber(tenantPublic, subject, nil, uc)
	}
	if err != nil {
		return nil, err
	}
	code := resp.OK
	if code == nil {
		return nil, errNoCode
	}
	if kind == users.KindEmail {
		err = s.sendEmail(emaildelivery.EmailType{PasswordlessLogin: &emaildelivery.PasswordlessLoginType{
			Email:            subject,
			UserInputCode:    &code.UserInputCode,
			CodeLifetime:     code.CodeLifetime,
			PreAuthSessionId: code.PreAuthSessionID,
			TenantId:         tenantPublic,
		}}, uc)
	} else {
		err = s.sendSMS(smsdelivery.SmsType{PasswordlessLogin: &smsdelivery.PasswordlessLoginType{
			PhoneNumber:      subject,
			UserInputCode:    &code.UserInputCode,
			CodeLifetime:     code.CodeLifetime,
			PreAuthSessionId: code.PreAuthSessionID,
			TenantId:         tenantPublic,
		}}, uc)
	}
	return code, err
}

func markLinking(uc supertokens.UserContext) {
	if uc == nil {
		return
	}
	if *uc == nil {
		*uc = map[string]interface{}{}
	}
	(*uc)[linkingContextKey] = true
}

func isLinking(uc supertokens.UserContext) bool {
	if uc == nil || *uc == nil {
		return false
	}
	linking, _ := (*uc)[linkingContextKey].(bool)
	return linking
}
