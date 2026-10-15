// Package sms defines outbound text messages and the providers that deliver them.
package sms

import "context"

type Message struct {
	To   string
	Body string
}

// Sender delivers a message to an E.164 number. Implementations must respect ctx and be safe for concurrent use.
type Sender interface {
	Send(ctx context.Context, m Message) error
}
