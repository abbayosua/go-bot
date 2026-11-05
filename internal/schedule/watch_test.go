package schedule

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"gobot/internal/store"
)

func TestFingerprintChangesWithContent(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, "ws")
	bot := "bot_1"
	if err := os.MkdirAll(filepath.Join(ws, bot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, bot, "notes.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := Fingerprint(ws, bot, "notes.txt")
	if before == "missing" || before == "refused" {
		t.Fatalf("fingerprint %q", before)
	}
	if err := os.WriteFile(filepath.Join(ws, bot, "notes.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	after := Fingerprint(ws, bot, "notes.txt")
	if before == after {
		t.Fatal("content change did not change the fingerprint")
	}
	if got := Fingerprint(ws, bot, "../escape"); got != "refused" {
		t.Fatalf("escape fingerprint %q, want refused", got)
	}
	if got := Fingerprint(ws, bot, "absent.txt"); got != "missing" {
		t.Fatalf("missing fingerprint %q", got)
	}
}

func TestWatchBaselineThenFire(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	bot, err := st.CreateBot(ctx, "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := st.CreateThread(ctx, bot.ID, "Chat")
	if err != nil {
		t.Fatal(err)
	}
	routine, err := st.CreateRoutine(ctx, bot.ID, thread.ID, "Watch", "report", "0 0 * * *", 0)
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	target := filepath.Join(ws, bot.ID)
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "data.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	scheduler := New(st, nil, log).WithWorkspaces(ws)
	// Seed the baseline directly (as the PUT endpoint would).
	if err := st.SetRoutineWatch(ctx, routine.ID, "data.txt", Fingerprint(ws, bot.ID, "data.txt")); err != nil {
		t.Fatal(err)
	}
	// No change: no run.
	scheduler.checkWatches(ctx)
	runs, err := st.ListRoutineRuns(ctx, routine.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("unchanged watch fired %d runs", len(runs))
	}
	// Change the file: one run fires with trigger watch.
	if err := os.WriteFile(filepath.Join(target, "data.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	scheduler.checkWatches(ctx)
	runs, err = st.ListRoutineRuns(ctx, routine.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	_ = runs
	all, err := st.ListRuns(ctx, thread.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	watchRuns := 0
	for _, run := range all {
		if run.Trigger == "watch" {
			watchRuns++
		}
	}
	if watchRuns != 1 {
		t.Fatalf("changed watch fired %d runs, want 1", watchRuns)
	}
}
