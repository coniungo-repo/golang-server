package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const (
	PostmarkEndpoint      = "https://api.postmarkapp.com/email"
	postmarkMaxResponse   = 64 << 10
	postmarkDefaultStream = "outbound"
)

// PostmarkConfig configures Postmark. From must be a confirmed sender signature or on a verified domain. Endpoint is for tests.
type PostmarkConfig struct {
	ServerToken string
	From        string
	Stream      string
	Endpoint    string
	Client      *http.Client
}

// Postmark sends email through Postmark's API. Errors carry the HTTP status and Postmark error code only, never the recipient or the token.
type Postmark struct {
	cfg PostmarkConfig
}

func NewPostmark(cfg PostmarkConfig) (*Postmark, error) {
	if cfg.ServerToken == "" || cfg.From == "" {
		return nil, errors.New("email: postmark needs a server token and a from address")
	}
	if cfg.Stream == "" {
		cfg.Stream = postmarkDefaultStream
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = PostmarkEndpoint
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{}
	}
	return &Postmark{cfg: cfg}, nil
}

type postmarkRequest struct {
	From          string
	To            string
	Subject       string
	TextBody      string `json:",omitempty"`
	HtmlBody      string `json:",omitempty"`
	MessageStream string
}

type postmarkResponse struct {
	ErrorCode int
}

func (p *Postmark) Send(ctx context.Context, m Message) error {
	body, err := json.Marshal(postmarkRequest{
		From: p.cfg.From, To: m.To, Subject: m.Subject, TextBody: m.Text, HtmlBody: m.HTML, MessageStream: p.cfg.Stream,
	})
	if err != nil {
		return fmt.Errorf("email: encoding postmark request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("email: building postmark request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Postmark-Server-Token", p.cfg.ServerToken)

	resp, err := p.cfg.Client.Do(req)
	if err != nil {
		return fmt.Errorf("email: postmark request failed: %w", err)
	}
	defer resp.Body.Close()

	var out postmarkResponse
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, postmarkMaxResponse)).Decode(&out)
	if resp.StatusCode != http.StatusOK || decodeErr != nil || out.ErrorCode != 0 {
		return fmt.Errorf("email: postmark returned status %d, error code %d", resp.StatusCode, out.ErrorCode)
	}
	return nil
}
