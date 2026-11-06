// Package msg carries external messaging: one adapter first (Telegram), then
// the rest. Inbound text is untrusted input: it enters through the same run
// loop and approval gate as a typed message, never straight to a tool.
package msg

import "context"

// Message is one inbound chat message.
type Message struct {
	ChatID  string
	From    string
	Text    string
	ReplyTo string
}

// Adapter sends outbound messages. Inbound arrives via webhook (Handler) or
// polling (Poller); both funnel into Ingest.
type Adapter interface {
	Name() string
	Send(ctx context.Context, chat, text string) error
}

// Ingest is what the server wires: persist the inbound text, start a run.
type Ingest func(ctx context.Context, botID, chat, from, text string) error
