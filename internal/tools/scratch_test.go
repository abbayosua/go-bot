package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"gobot/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestScratchpadRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	bot, err := st.CreateBot(ctx, "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	note := &noteTool{store: st}
	raw, _ := json.Marshal(map[string]string{"text": "check the logs"})
	if _, err := note.RunFor(ctx, bot.ID, raw); err != nil {
		t.Fatal(err)
	}
	if block := ScratchBlock(ctx, st, bot.ID); block == "" {
		t.Fatal("scratch block empty after note")
	}
	notes := &notesTool{store: st}
	out, err := notes.RunFor(ctx, bot.ID, nil)
	if err != nil || out == "no notes yet" {
		t.Fatalf("notes %q err %v", out, err)
	}
	if _, err := (&clearNotesTool{store: st}).RunFor(ctx, bot.ID, nil); err != nil {
		t.Fatal(err)
	}
	if block := ScratchBlock(ctx, st, bot.ID); block != "" {
		t.Fatalf("scratch not cleared: %q", block)
	}
}

func TestAskBotCitesSource(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	alice, err := st.CreateBot(ctx, "Alice", "", "")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := st.CreateBot(ctx, "Bob", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMemory(ctx, bob.ID, "the deploy port is 8222"); err != nil {
		t.Fatal(err)
	}
	tool := &askBotTool{store: st}
	raw, _ := json.Marshal(map[string]string{"bot": "Bob", "question": "deploy port"})
	answer, err := tool.RunFor(ctx, alice.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	if answer == "" || !contains(answer, "8222") || !contains(answer, "Bob") {
		t.Fatalf("answer %q misses fact or source", answer)
	}
	if _, err := tool.RunFor(ctx, alice.ID, mustJSON(map[string]string{"bot": "Alice", "question": "x"})); err == nil {
		t.Fatal("self-ask should fail")
	}
	if _, err := tool.RunFor(ctx, alice.ID, mustJSON(map[string]string{"bot": "Ghost", "question": "x"})); err == nil {
		t.Fatal("unknown bot should fail")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func mustJSON(v map[string]string) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}
