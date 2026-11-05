package store

import (
	"context"
	"path/filepath"
	"testing"
)

func openMemory(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestMemoriesScopeAndSearch(t *testing.T) {
	ctx := context.Background()
	st := openMemory(t)

	if _, err := st.AddMemory(ctx, "", "Deploy on Fridays only."); err != nil {
		t.Fatalf("shared: %v", err)
	}
	if _, err := st.AddMemory(ctx, "bot_a", "SMTP relay runs on port 2525."); err != nil {
		t.Fatalf("bot: %v", err)
	}
	if _, err := st.AddMemory(ctx, "bot_b", "bot_b uses the nightly retrain job."); err != nil {
		t.Fatalf("other bot: %v", err)
	}

	visible, err := st.ListMemories(ctx, "bot_a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 2 {
		t.Fatalf("bot_a sees %d memories, want shared + own", len(visible))
	}
	if visible[0].BotID != "" && visible[1].BotID != "" {
		t.Error("shared memory should sort before bot memory")
	}

	results, err := st.SearchMemories(ctx, "bot_a", "SMTP PORT", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].BotID != "bot_a" {
		t.Errorf("search %+v, want the smtp fact", results)
	}

	results, err = st.SearchMemories(ctx, "bot_a", "nightly retrain", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("another bot's memory leaked: %+v", results)
	}

	if err := st.DeleteMemory(ctx, visible[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	visible, err = st.ListMemories(ctx, "bot_a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 {
		t.Errorf("after delete bot_a sees %d, want 1", len(visible))
	}
}

func TestKnowledgeScopeAndSearch(t *testing.T) {
	ctx := context.Background()
	st := openMemory(t)

	if _, err := st.AddKnowledge(ctx, "bot_a", "Deploy notes", "Run migrate before rolling the service."); err != nil {
		t.Fatal(err)
	}
	entries, err := st.SearchKnowledge(ctx, "bot_a", "migrate service", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Title != "Deploy notes" {
		t.Errorf("search %+v", entries)
	}
	entries, err = st.SearchKnowledge(ctx, "bot_b", "migrate", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("bot_b sees bot_a knowledge: %+v", entries)
	}
}

func TestSkills(t *testing.T) {
	ctx := context.Background()
	st := openMemory(t)

	if _, err := st.CreateSkill(ctx, "review", "Code review", "Check tests first."); err != nil {
		t.Fatal(err)
	}
	skill, err := st.GetSkill(ctx, "review")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if skill.Body != "Check tests first." {
		t.Errorf("body %q", skill.Body)
	}
	if _, err := st.GetSkill(ctx, "missing"); err == nil {
		t.Error("missing skill was found")
	}
	skills, err := st.ListSkills(ctx)
	if err != nil || len(skills) != 1 {
		t.Errorf("list %+v err %v", skills, err)
	}
	if err := st.DeleteSkill(ctx, skill.ID); err != nil {
		t.Fatal(err)
	}
}
