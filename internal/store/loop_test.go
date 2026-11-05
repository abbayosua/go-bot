package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func openLoop(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "loop.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestCompactionKeepsTheNewestWithinBudget(t *testing.T) {
	history := []Message{}
	for i := 0; i < 10; i++ {
		history = append(history, Message{ID: "m" + string('a'+rune(i)), Role: "user", Content: strings.Repeat("x", 100)})
	}
	kept, dropped := CompactHistory(history, 500, 4)
	if len(kept) != 4 || len(dropped) != 6 {
		t.Fatalf("kept %d dropped %d, want 4 and 6", len(kept), len(dropped))
	}
	if kept[0].ID != "mg" || dropped[0].ID != "ma" {
		t.Errorf("wrong window: kept[0]=%s dropped[0]=%s", kept[0].ID, dropped[0].ID)
	}
}

func TestCompactionDoesNothingShortThreads(t *testing.T) {
	history := []Message{{Role: "user", Content: "hi"}}
	kept, dropped := CompactHistory(history, 60000, 40)
	if len(kept) != 1 || len(dropped) != 0 {
		t.Errorf("short history was touched: kept %d dropped %d", len(kept), len(dropped))
	}
}

func TestSummarizeDroppedKeepsNamesAndFirstLines(t *testing.T) {
	history := []Message{
		{Role: "user", Content: "write the file\nsecond line"},
		{Role: "assistant", ToolCalls: `[{"id":"1","function":{"name":"shell","arguments":"{}"}}]`},
		{Role: "tool", ToolName: "shell", ToolCallID: "1", Content: "error: refused"},
	}
	summary := SummarizeDropped(history)
	for _, want := range []string{"user: write the file", "assistant tool calls", "tool result (shell)"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary %q misses %q", summary, want)
		}
	}
}

func TestInstructionsRoundTripAndUndo(t *testing.T) {
	ctx := context.Background()
	st := openLoop(t)
	bot, err := st.CreateBot(ctx, "B", "first", "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := st.AddInstructionVersion(ctx, bot.ID, "second", "chat")
	if err != nil {
		t.Fatal(err)
	}
	if first.PrevInstructions != "first" {
		t.Fatalf("proposal snapshot %q, want first", first.PrevInstructions)
	}
	if err := st.UpdateBot(ctx, bot.ID, bot.Name, "second", bot.Model); err != nil {
		t.Fatal(err)
	}
	second, err := st.AddInstructionVersion(ctx, bot.ID, "third", "chat")
	if err != nil {
		t.Fatal(err)
	}
	// The sequence is monotonic regardless of timestamp ties or random IDs.
	if second.Seq <= first.Seq {
		t.Fatalf("seq not monotonic: first %d second %d", first.Seq, second.Seq)
	}
	versions, err := st.ListInstructionVersions(ctx, bot.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions %+v", versions)
	}
	// Newest first: versions[0] is "third", versions[1] is "second".
	if versions[0].Seq != second.Seq || versions[1].Seq != first.Seq {
		t.Fatalf("list order ignores seq: %+v", versions)
	}
	previous, err := st.UndoInstructionVersion(ctx, versions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if previous != "second" {
		t.Errorf("undo of newer row returned %q, want second", previous)
	}
	previous, err = st.UndoInstructionVersion(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Undo restores the snapshot captured at proposal time, regardless of
	// timestamp ties or random ID ordering.
	if previous != "first" {
		t.Errorf("undo returned %q, want first", previous)
	}
	versions, err = st.ListInstructionVersions(ctx, bot.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range versions {
		if !version.Undone {
			t.Errorf("undo was not recorded: %+v", versions)
			break
		}
	}
}

func TestInstructionSeqSurvivesMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	bot, err := st.CreateBot(ctx, "B", "orig", "")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a database written before the seq column existed.
	if _, err := st.db.Exec(`INSERT INTO instruction_versions (id, bot_id, instructions, source, prev_instructions, seq, created_at)
		VALUES ('ins_legacy1', ?, 'v1', 'chat', 'orig', 0, 1), ('ins_legacy2', ?, 'v2', 'chat', 'v1', 0, 1)`,
		bot.ID, bot.ID); err != nil {
		t.Fatal(err)
	}
	st.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen legacy db: %v", err)
	}
	defer reopened.Close()
	versions, err := reopened.ListInstructionVersions(ctx, bot.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].Instructions != "v2" || versions[1].Instructions != "v1" {
		t.Fatalf("backfill order wrong: %+v", versions)
	}
	if versions[0].Seq <= versions[1].Seq || versions[1].Seq <= 0 {
		t.Fatalf("backfill seq not monotonic: %+v", versions)
	}
	fresh, err := reopened.AddInstructionVersion(ctx, bot.ID, "v3", "chat")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Seq <= versions[0].Seq {
		t.Fatalf("post-migration seq %d not above backfilled %d", fresh.Seq, versions[0].Seq)
	}
}

func TestFeedRowsPairRequestsWithResults(t *testing.T) {
	history := []Message{
		{ID: "a1", Role: "assistant", ToolCalls: `[{"id":"c1","function":{"name":"shell","arguments":"{\"command\":\"ls\"}"}}]`, Content: "first"},
		{ID: "t1", Role: "tool", ToolCallID: "c1", ToolName: "shell", Content: "a.txt"},
		{ID: "a2", Role: "assistant", ToolCalls: `[{"id":"c2","function":{"name":"shell","arguments":"{\"command\":\"ls /\"}"}}]`, Content: "second"},
		{ID: "t2", Role: "tool", ToolCallID: "c2", ToolName: "shell", Content: "error: refused"},
		{ID: "u1", Role: "user", Content: "again"},
	}
	rows := Feed(history)
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}
	if rows[0].Summary != "shell ls" {
		t.Errorf("row 0 summary %q", rows[0].Summary)
	}
	if rows[2].Summary != "shell ls /" {
		t.Errorf("row 2 summary %q", rows[2].Summary)
	}
	if rows[0].Error || !rows[3].Error {
		t.Error("error flags are wrong")
	}
	if rows[3].Output != "error: refused" {
		t.Errorf("refusal output %q", rows[3].Output)
	}
	if rows[1].Output != "a.txt" || !rows[0].Call || rows[0].Tool != "shell" {
		t.Errorf("row 0 %+v row 1 %+v", rows[0], rows[1])
	}
}
