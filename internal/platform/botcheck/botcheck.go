// Package botcheck confirms that a request came from a person before the API spends money on it, using Cloudflare Turnstile.
package botcheck

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

const (
	TurnstileEndpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	maxTokenLen       = 2048
	maxResponse       = 64 << 10
	attempts          = 2
)

// ErrFailed means the token was missing, invalid, expired or already used. The client should run the challenge again.
var ErrFailed = errors.New("botcheck: challenge failed")

var clientErrors = []string{"missing-input-response", "invalid-input-response", "timeout-or-duplicate"}

// Verifier checks a challenge token. Any error other than ErrFailed means the check could not be done, and callers must refuse rather than let the request through.
type Verifier interface {
	Verify(ctx context.Context, token, remoteIP string) error
}

// Off accepts every request. Use it only where no client can produce tokens yet.
type Off struct{}

func (Off) Verify(context.Context, string, string) error { return nil }

// TurnstileConfig configures Turnstile. Endpoint and Client are for tests.
type TurnstileConfig struct {
	Secret   string
	Endpoint string
	Client   *http.Client
}

// Turnstile verifies tokens with Cloudflare's siteverify API. Errors never include the token or the secret.
type Turnstile struct {
	cfg TurnstileConfig
}

func NewTurnstile(cfg TurnstileConfig) (*Turnstile, error) {
	if cfg.Secret == "" {
		return nil, errors.New("botcheck: turnstile needs a secret key")
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = TurnstileEndpoint
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{}
	}
	return &Turnstile{cfg: cfg}, nil
}

type siteverifyResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

// Verify checks token once, retrying a single time with the same idempotency key if Cloudflare could not answer.
func (t *Turnstile) Verify(ctx context.Context, token, remoteIP string) error {
	if token == "" || len(token) > maxTokenLen {
		return ErrFailed
	}
	key, err := idempotencyKey()
	if err != nil {
		return err
	}
	form := url.Values{"secret": {t.cfg.Secret}, "response": {token}, "idempotency_key": {key}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	var lastErr error
	for range attempts {
		var out siteverifyResponse
		status, err := t.post(ctx, form, &out)
		switch {
		case err != nil:
			lastErr = err
		case out.Success:
			return nil
		case slices.ContainsFunc(out.ErrorCodes, func(c string) bool { return slices.Contains(clientErrors, c) }):
			return ErrFailed
		case status >= 500 || slices.Contains(out.ErrorCodes, "internal-error"):
			lastErr = fmt.Errorf("botcheck: turnstile unavailable (status %d, codes %s)", status, strings.Join(out.ErrorCodes, ","))
		default:
			return fmt.Errorf("botcheck: turnstile rejected the request (status %d, codes %s)", status, strings.Join(out.ErrorCodes, ","))
		}
		if ctx.Err() != nil {
			break
		}
	}
	return lastErr
}

func (t *Turnstile) post(ctx context.Context, form url.Values, out *siteverifyResponse) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, fmt.Errorf("botcheck: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := t.cfg.Client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("botcheck: turnstile request failed: %w", err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponse)).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("botcheck: turnstile returned status %d with an unreadable body", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func idempotencyKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("botcheck: idempotency key: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}
