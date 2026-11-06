package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"gobot/internal/core"
	"gobot/internal/store"
)

func openMemory(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "memory-block.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestBlockShapesSharedFirst(t *testing.T) {
	ctx := context.Background()
	st := openMemory(t)
	st.AddMemory(ctx, "", "User deploys on Fridays only.")
	st.AddMemory(ctx, "bot_a", "Project X uses the staging relay.")
	st.AddMemory(ctx, "bot_b", "Should stay invisible to bot_a.")

	block := Block(ctx, st.Memories(), "bot_a")
	if !strings.Contains(block, "Shared facts:") || !strings.Contains(block, "about this bot") {
		t.Errorf("block has no sections:\n%s", block)
	}
	if strings.Contains(block, "stay invisible") {
		t.Error("another bot's memory leaked into the block")
	}
	shared := strings.Index(block, "Fridays")
	personal := strings.Index(block, "staging relay")
	if shared < 0 || personal < 0 || shared > personal {
		t.Errorf("shared facts must come first:\n%s", block)
	}
}

func TestBlockRespectsBudget(t *testing.T) {
	ctx := context.Background()
	st := openMemory(t)
	for i := 0; i < 40; i++ {
		st.AddMemory(ctx, "bot_a", strings.Repeat("x", 300))
	}
	block := Block(ctx, st.Memories(), "bot_a")
	if len(block) > MaxMemoryChars+200 {
		t.Errorf("block is %d chars, budget is %d", len(block), MaxMemoryChars)
	}
}

func TestEmptyBlock(t *testing.T) {
	ctx := context.Background()
	st := openMemory(t)
	if Block(ctx, st.Memories(), "bot_a") != "" {
		t.Error("empty memory should produce an empty block")
	}
}

func TestInvokeSkillWithKnowledge(t *testing.T) {
	ctx := context.Background()
	st := openMemory(t)
	_, err := st.CreateSkill(ctx, "review", "Code review", "Check tests first, then style.")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.AddKnowledge(ctx, "bot_a", "Review bar", "Style follows the repo guide.")
	if err != nil {
		t.Fatal(err)
	}

	out, err := Invoke(ctx, st, "bot_a", "/review", "review bar style")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Check tests first") {
		t.Errorf("playbook missing:\n%s", out)
	}
	if !strings.Contains(out, "Review bar") {
		t.Errorf("related knowledge missing:\n%s", out)
	}

	out, err = Invoke(ctx, st, "bot_a", "/missing", "anything")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Unknown skill") || !strings.Contains(out, "/review") {
		t.Errorf("unknown skill should list what exists:\n%s", out)
	}

	out, err = Invoke(ctx, st, "bot_a", "/review", "unrelated topic")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No matching knowledge") {
		t.Errorf("empty knowledge should say so:\n%s", out)
	}
}

func TestSkillNames(t *testing.T) {
	if SkillNames(nil) != "" {
		t.Error("no skills should mean no names")
	}
	if got := SkillNames([]store.Skill{{Name: "review"}, {Name: "deploy"}}); got != "/review, /deploy" {
		t.Errorf("names %q", got)
	}
}

func TestBlockReadsThroughChain(t *testing.T) {
	ctx := context.Background()
	st := openMemory(t)
	bot, err := st.CreateBot(ctx, "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMemory(ctx, bot.ID, "local fact"); err != nil {
		t.Fatal(err)
	}
	external := &stubProvider{items: []core.Memory{{ID: "x", BotID: bot.ID, Content: "external fact", CreatedAt: 9999999999}}}
	chain := core.NewChain(st.Memories(), external)
	block := Block(ctx, chain, bot.ID)
	if !strings.Contains(block, "local fact") || !strings.Contains(block, "external fact") {
		t.Fatalf("block misses a provider: %q", block)
	}
	local := Block(ctx, st.Memories(), bot.ID)
	if !strings.Contains(local, "local fact") || strings.Contains(local, "external fact") {
		t.Fatalf("builtin block wrong: %q", local)
	}
}

type stubProvider struct {
	items []core.Memory
}

func (s *stubProvider) Name() string { return "stub" }

func (s *stubProvider) Add(_ context.Context, botID, content string) (core.Memory, error) {
	return core.Memory{BotID: botID, Content: content}, nil
}

func (s *stubProvider) List(_ context.Context, _ string, _ int) ([]core.Memory, error) {
	return s.items, nil
}

func (s *stubProvider) Search(_ context.Context, _, _ string, _ int) ([]core.Memory, error) {
	return s.items, nil
}
