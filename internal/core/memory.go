// Package core holds provider-neutral contracts: the seams where an external
// service can plug in without the agent loop learning a new code path.
package core

import "context"

// Memory is one durable fact, shared install-wide when BotID is empty.
type Memory struct {
	ID        string
	BotID     string
	Content   string
	CreatedAt int64
}

// MemoryProvider stores and retrieves durable facts. The builtin SQLite
// adapter lives in internal/store; an external service is another adapter
// behind this interface, never a special case in the agent loop.
type MemoryProvider interface {
	// Name identifies the provider in logs and the UI.
	Name() string
	// Add stores one fact for a bot (empty botID means shared).
	Add(ctx context.Context, botID, content string) (Memory, error)
	// List returns the newest facts for a bot, shared ones included.
	List(ctx context.Context, botID string, limit int) ([]Memory, error)
	// Search matches a query against a bot's facts, newest first.
	Search(ctx context.Context, botID, query string, limit int) ([]Memory, error)
}
