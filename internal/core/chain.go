package core

import (
	"context"
	"sort"
)

// Chain fans reads out to every provider and merges newest-first. Writes go
// to the primary only: external mirrors are read replicas, never the source
// of truth for a new fact.
type Chain struct {
	primary MemoryProvider
	extra   []MemoryProvider
}

// NewChain builds a provider chain: primary takes writes, every provider is
// read. Pass no extra providers for the builtin-only install.
func NewChain(primary MemoryProvider, extra ...MemoryProvider) *Chain {
	return &Chain{primary: primary, extra: extra}
}

func (c *Chain) Name() string { return "chain(" + c.primary.Name() + ")" }

func (c *Chain) Add(ctx context.Context, botID, content string) (Memory, error) {
	return c.primary.Add(ctx, botID, content)
}

func (c *Chain) List(ctx context.Context, botID string, limit int) ([]Memory, error) {
	return c.merge(ctx, limit, func(p MemoryProvider) ([]Memory, error) {
		return p.List(ctx, botID, limit)
	})
}

func (c *Chain) Search(ctx context.Context, botID, query string, limit int) ([]Memory, error) {
	return c.merge(ctx, limit, func(p MemoryProvider) ([]Memory, error) {
		return p.Search(ctx, botID, query, limit)
	})
}

func (c *Chain) merge(ctx context.Context, limit int, read func(MemoryProvider) ([]Memory, error)) ([]Memory, error) {
	var out []Memory
	for _, provider := range append([]MemoryProvider{c.primary}, c.extra...) {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		items, err := read(provider)
		if err != nil {
			continue
		}
		out = append(out, items...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return out[i].ID > out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
