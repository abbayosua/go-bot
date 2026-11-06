package core

import (
	"context"
	"errors"
	"testing"
)

type fakeProvider struct {
	name  string
	items []Memory
	err   bool
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Add(_ context.Context, botID, content string) (Memory, error) {
	return Memory{ID: "fake", BotID: botID, Content: content}, nil
}

func (f *fakeProvider) List(_ context.Context, _ string, _ int) ([]Memory, error) {
	if f.err {
		return nil, errors.New("down")
	}
	return f.items, nil
}

func (f *fakeProvider) Search(_ context.Context, _, _ string, _ int) ([]Memory, error) {
	if f.err {
		return nil, errors.New("down")
	}
	return f.items, nil
}

func TestChainMergesNewestFirst(t *testing.T) {
	ctx := context.Background()
	primary := &fakeProvider{name: "local", items: []Memory{{ID: "a", Content: "old", CreatedAt: 1}}}
	external := &fakeProvider{name: "svc", items: []Memory{{ID: "b", Content: "new", CreatedAt: 2}}}
	chain := NewChain(primary, external)
	got, err := chain.List(ctx, "bot", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" {
		t.Fatalf("merge order %+v", got)
	}
	capped, err := chain.List(ctx, "bot", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(capped) != 1 || capped[0].ID != "b" {
		t.Fatalf("limit %+v", capped)
	}
}

func TestChainSurvivesProviderFailure(t *testing.T) {
	ctx := context.Background()
	primary := &fakeProvider{name: "local", items: []Memory{{ID: "a", Content: "old", CreatedAt: 1}}}
	dead := &fakeProvider{name: "svc", err: true}
	chain := NewChain(primary, dead)
	got, err := chain.Search(ctx, "bot", "q", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("fallback %+v", got)
	}
}

func TestChainWritesGoToPrimary(t *testing.T) {
	ctx := context.Background()
	chain := NewChain(&fakeProvider{name: "local"})
	got, err := chain.Add(ctx, "bot", "fact")
	if err != nil {
		t.Fatal(err)
	}
	if got.BotID != "bot" || got.Content != "fact" {
		t.Fatalf("write %+v", got)
	}
	if chain.Name() == "" {
		t.Fatal("chain needs a name")
	}
}
