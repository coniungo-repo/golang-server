package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"math"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/sms"
	"github.com/supertokens/supertokens-golang/ingredients/emaildelivery"
	"github.com/supertokens/supertokens-golang/ingredients/smsdelivery"
	"github.com/supertokens/supertokens-golang/supertokens"
)

const deliveryTimeout = 10 * time.Second

type codeData struct {
	AppName string
	Code    string
	Minutes int
}

var (
	emailSubject = texttemplate.Must(texttemplate.New("subject").Parse(`{{.Code}} is your {{.AppName}} sign-in code`))
	emailText    = texttemplate.Must(texttemplate.New("text").Parse(`Your {{.AppName}} sign-in code is {{.Code}}.

It expires in {{.Minutes}} minutes. If you didn't try to sign in, you can ignore this email. Never share this code with anyone.
`))
	emailHTML = htmltemplate.Must(htmltemplate.New("html").Parse(`<!doctype html>
<html lang="en">
<body style="margin:0;padding:24px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#111;background:#fff">
<p style="margin:0 0 16px">Your {{.AppName}} sign-in code is</p>
<p style="margin:0 0 16px;font-size:32px;font-weight:700;letter-spacing:6px">{{.Code}}</p>
<p style="margin:0 0 8px;color:#555">It expires in {{.Minutes}} minutes. If you didn't try to sign in, you can ignore this email.</p>
<p style="margin:0;color:#555">Never share this code with anyone.</p>
</body>
</html>
`))
	smsBody = texttemplate.Must(texttemplate.New("sms").Parse(`{{.Code}} is your {{.AppName}} code. It expires in {{.Minutes}} min. Don't share it with anyone.`))
)

var errNoCode = errors.New("auth: passwordless message has no user input code")

func (s *Service) sendEmail(input emaildelivery.EmailType, _ supertokens.UserContext) error {
	in := input.PasswordlessLogin
	if in == nil {
		return errors.New("auth: unsupported email type")
	}
	data, err := s.codeData(in.UserInputCode, in.CodeLifetime)
	if err != nil {
		return err
	}

	msg := email.Message{To: in.Email}
	if msg.Subject, err = render(emailSubject, data); err != nil {
		return err
	}
	if msg.Text, err = render(emailText, data); err != nil {
		return err
	}
	var html bytes.Buffer
	if err := emailHTML.Execute(&html, data); err != nil {
		return fmt.Errorf("auth: rendering email: %w", err)
	}
	msg.HTML = html.String()

	ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
	defer cancel()
	if err := s.deps.Email.Send(ctx, msg); err != nil {
		return fmt.Errorf("auth: sending sign-in email: %w", err)
	}
	return nil
}

func (s *Service) sendSMS(input smsdelivery.SmsType, _ supertokens.UserContext) error {
	in := input.PasswordlessLogin
	if in == nil {
		return errors.New("auth: unsupported sms type")
	}
	data, err := s.codeData(in.UserInputCode, in.CodeLifetime)
	if err != nil {
		return err
	}
	body, err := render(smsBody, data)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
	defer cancel()
	if err := s.deps.SMS.Send(ctx, sms.Message{To: in.PhoneNumber, Body: body}); err != nil {
		return fmt.Errorf("auth: sending sign-in sms: %w", err)
	}
	return nil
}

func (s *Service) codeData(code *string, lifetimeMillis uint64) (codeData, error) {
	if code == nil || *code == "" {
		return codeData{}, errNoCode
	}
	return codeData{
		AppName: s.deps.Config.AppName,
		Code:    *code,
		Minutes: int(math.Ceil(float64(lifetimeMillis) / float64(time.Minute.Milliseconds()))),
	}, nil
}

func render(t *texttemplate.Template, data codeData) (string, error) {
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return "", fmt.Errorf("auth: rendering %s: %w", t.Name(), err)
	}
	return b.String(), nil
}
