package sms

import (
	"context"
	"log/slog"
)

// LogSender writes messages to the log instead of sending them. Config rejects it in production.
type LogSender struct {
	Log *slog.Logger
}

func (s LogSender) Send(ctx context.Context, m Message) error {
	s.Log.InfoContext(ctx, "sms not sent (log provider)", "to", m.To, "body", m.Body)
	return nil
}
