package store

import (
	"context"

	"gobot/internal/core"
)

// SQLiteProvider is the builtin MemoryProvider adapter: the memories table
// behind the neutral core.MemoryProvider contract.
type SQLiteProvider struct {
	store *Store
}

// Memories returns the builtin adapter for one store.
func (s *Store) Memories() *SQLiteProvider { return &SQLiteProvider{store: s} }

// Compile-time check: the builtin store satisfies the neutral contract.
var _ core.MemoryProvider = (*SQLiteProvider)(nil)

func (p *SQLiteProvider) Name() string { return "sqlite" }

func toCoreMemory(m Memory) core.Memory {
	return core.Memory{ID: m.ID, BotID: m.BotID, Content: m.Content, CreatedAt: m.CreatedAt}
}

func (p *SQLiteProvider) Add(ctx context.Context, botID, content string) (core.Memory, error) {
	memory, err := p.store.AddMemory(ctx, botID, content)
	return toCoreMemory(memory), err
}

func (p *SQLiteProvider) List(ctx context.Context, botID string, limit int) ([]core.Memory, error) {
	memories, err := p.store.ListMemories(ctx, botID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]core.Memory, 0, len(memories))
	for _, memory := range memories {
		out = append(out, toCoreMemory(memory))
	}
	return out, nil
}

func (p *SQLiteProvider) Search(ctx context.Context, botID, query string, limit int) ([]core.Memory, error) {
	memories, err := p.store.SearchMemories(ctx, botID, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]core.Memory, 0, len(memories))
	for _, memory := range memories {
		out = append(out, toCoreMemory(memory))
	}
	return out, nil
}
