package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	termiiSendPath    = "/api/sms/send"
	termiiMaxResponse = 64 << 10

	TermiiChannelDND     = "dnd"
	TermiiChannelGeneric = "generic"
)

// TermiiConfig configures Termii. BaseURL is the account's API base URL from the Termii dashboard. Use the dnd channel for codes: generic does not reach numbers on Do-Not-Disturb.
type TermiiConfig struct {
	APIKey  string
	BaseURL string
	Sender  string
	Channel string
	Client  *http.Client
}

// Termii sends SMS through Termii's API. Errors carry the HTTP status and Termii's code only, never the number, the message or the key.
type Termii struct {
	cfg      TermiiConfig
	endpoint string
}

func NewTermii(cfg TermiiConfig) (*Termii, error) {
	if cfg.APIKey == "" || cfg.BaseURL == "" || cfg.Sender == "" {
		return nil, errors.New("sms: termii needs an API key, a base URL and a sender ID")
	}
	if cfg.Channel != TermiiChannelDND && cfg.Channel != TermiiChannelGeneric {
		return nil, fmt.Errorf("sms: termii channel must be %q or %q", TermiiChannelDND, TermiiChannelGeneric)
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{}
	}
	return &Termii{cfg: cfg, endpoint: strings.TrimSuffix(cfg.BaseURL, "/") + termiiSendPath}, nil
}

type termiiRequest struct {
	APIKey  string `json:"api_key"`
	To      string `json:"to"`
	From    string `json:"from"`
	SMS     string `json:"sms"`
	Type    string `json:"type"`
	Channel string `json:"channel"`
}

type termiiResponse struct {
	Code      string `json:"code"`
	MessageID string `json:"message_id"`
}

func (t *Termii) Send(ctx context.Context, m Message) error {
	body, err := json.Marshal(termiiRequest{
		APIKey: t.cfg.APIKey, To: strings.TrimPrefix(m.To, "+"), From: t.cfg.Sender, SMS: m.Body, Type: "plain", Channel: t.cfg.Channel,
	})
	if err != nil {
		return fmt.Errorf("sms: encoding termii request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sms: building termii request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.cfg.Client.Do(req)
	if err != nil {
		return fmt.Errorf("sms: termii request failed: %w", err)
	}
	defer resp.Body.Close()

	var out termiiResponse
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, termiiMaxResponse)).Decode(&out)
	if resp.StatusCode < 200 || resp.StatusCode > 299 || decodeErr != nil || (out.Code != "" && out.Code != "ok") || out.MessageID == "" {
		return fmt.Errorf("sms: termii returned status %d, code %q", resp.StatusCode, out.Code)
	}
	return nil
}
