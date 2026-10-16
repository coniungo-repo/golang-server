package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/coniungo-repo/golang-server/internal/platform/botcheck"
	"github.com/coniungo-repo/golang-server/internal/platform/metrics"
	"github.com/coniungo-repo/golang-server/internal/platform/request"
)

// BotCheckHeader carries the Turnstile token on requests that send a code.
const BotCheckHeader = "X-Turnstile-Token"

const (
	botCheckFailedMessage      = "Please complete the check and try again."
	botCheckUnavailableMessage = "We couldn't check that right now. Try again in a moment."
)

// checkHuman runs the bot check for a request that is about to send a code, returning the message to refuse it with, or "" to go ahead. It fails closed: if the check cannot be done, the code is not sent.
func (s *Service) checkHuman(r *http.Request) string {
	ctx, cancel := context.WithTimeout(r.Context(), s.deps.BotCheckTimeout)
	defer cancel()
	err := s.deps.BotCheck.Verify(ctx, r.Header.Get(BotCheckHeader), request.ClientIP(r, s.deps.ClientIPHeader))
	switch {
	case err == nil:
		s.deps.Metrics.BotCheck(metrics.ResultOK)
		return ""
	case errors.Is(err, botcheck.ErrFailed):
		s.deps.Metrics.BotCheck(metrics.BotCheckFailed)
		return botCheckFailedMessage
	default:
		s.deps.Metrics.BotCheck(metrics.ResultError)
		s.deps.Log.ErrorContext(r.Context(), "bot check unavailable", "err", err)
		return botCheckUnavailableMessage
	}
}
