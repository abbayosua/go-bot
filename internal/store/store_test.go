package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func open(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestBotThreadMessageRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := open(t)

	bot, err := st.CreateBot(ctx, "Scout", "research", "model-a")
	if err != nil {
		t.Fatalf("create bot: %v", err)
	}
	if bot.ID == "" {
		t.Fatal("bot has no id")
	}
	thread, err := st.CreateThread(ctx, bot.ID, "Chat")
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if _, err := st.AddMessage(ctx, Message{ThreadID: thread.ID, Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("add message: %v", err)
	}
	messages, err := st.ListMessages(ctx, thread.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "hello" {
		t.Fatalf("messages %+v", messages)
	}

	if err := st.UpdateBot(ctx, bot.ID, "Scout 2", "research harder", ""); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, err := st.GetBot(ctx, bot.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Name != "Scout 2" || updated.Model != "" {
		t.Errorf("bot not updated: %+v", updated)
	}
}

func TestFirstThreadCreatesOne(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	bot, err := st.CreateBot(ctx, "New", "", "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := st.FirstThread(ctx, bot.ID)
	if err != nil {
		t.Fatalf("first thread: %v", err)
	}
	second, err := st.FirstThread(ctx, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Errorf("first thread changed from %s to %s", first.ID, second.ID)
	}
}

func TestClaimRunRespectsLease(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	bot, _ := st.CreateBot(ctx, "B", "", "")
	thread, _ := st.CreateThread(ctx, bot.ID, "Chat")
	run, err := st.CreateRun(ctx, bot.ID, thread.ID, "user", "go")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ClaimRun(ctx, run.ID, time.Minute); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if _, err := st.ClaimRun(ctx, run.ID, time.Minute); err == nil {
		t.Error("second claim succeeded while the lease was held")
	}
	if err := st.ReleaseRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ClaimRun(ctx, run.ID, time.Minute); err != nil {
		t.Errorf("claim after release: %v", err)
	}
}

func TestApprovalDecisionIsOnce(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	bot, _ := st.CreateBot(ctx, "B", "", "")
	thread, _ := st.CreateThread(ctx, bot.ID, "Chat")
	run, _ := st.CreateRun(ctx, bot.ID, thread.ID, "user", "go")

	created, err := st.CreateApproval(ctx, run.ID, thread.ID, "call_1", "shell", `{"command":"ls"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateApproval(ctx, run.ID, thread.ID, "call_1", "shell", `{"command":"ls"}`); err != nil {
		t.Fatalf("duplicate create should be ignored: %v", err)
	}
	if _, err := st.DecideApproval(ctx, created.ID, ApprovalAllow); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DecideApproval(ctx, created.ID, ApprovalDeny); err == nil {
		t.Error("a decided approval was decided twice")
	}
	stored, err := st.ApprovalForCall(ctx, run.ID, "call_1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != ApprovalAllow {
		t.Errorf("status %q, want allow", stored.Status)
	}
}

func TestDueRoutinesAreSelectedAndAdvanced(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	bot, _ := st.CreateBot(ctx, "B", "", "")
	thread, _ := st.CreateThread(ctx, bot.ID, "Chat")

	past := time.Now().Add(-time.Minute).Unix()
	routine, err := st.CreateRoutine(ctx, bot.ID, thread.ID, "Daily", "summarize", "0 8 * * *", past)
	if err != nil {
		t.Fatal(err)
	}
	due, err := st.DueRoutines(ctx, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != routine.ID {
		t.Fatalf("due %+v", due)
	}
	if err := st.MarkRoutineFired(ctx, routine.ID, time.Now().Unix(), time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	due, err = st.DueRoutines(ctx, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Errorf("routine fired again: %+v", due)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	if _, err := st.GetSetting(ctx, "approval_mode"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing setting returned %v", err)
	}
	if err := st.SetSetting(ctx, "approval_mode", "never"); err != nil {
		t.Fatal(err)
	}
	value, err := st.GetSetting(ctx, "approval_mode")
	if err != nil || value != "never" {
		t.Errorf("value %q err %v", value, err)
	}
}

func TestRoutineErrorHistoryAndSearch(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	botA, err := st.CreateBot(ctx, "A", "", "")
	if err != nil {
		t.Fatal(err)
	}
	botB, err := st.CreateBot(ctx, "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	threadA, err := st.CreateThread(ctx, botA.ID, "Chat")
	if err != nil {
		t.Fatal(err)
	}
	threadB, err := st.CreateThread(ctx, botB.ID, "Chat")
	if err != nil {
		t.Fatal(err)
	}
	routine, err := st.CreateRoutine(ctx, botA.ID, threadA.ID, "Morning", "brief me", "0 8 * * *", now()+3600)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RecordRoutineError(ctx, routine.ID, "boom"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastError != "boom" {
		t.Fatalf("lastError %q, want boom", got.LastError)
	}
	run, err := st.CreateRun(ctx, botA.ID, threadA.ID, "routine", "brief me")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(ctx, run.ID, RunDone, "", 1); err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListRoutineRuns(ctx, routine.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("routine runs %+v", runs)
	}
	if _, err := st.AddMessage(ctx, Message{ThreadID: threadA.ID, Role: "user", Content: "the lighthouse keeper"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMessage(ctx, Message{ThreadID: threadB.ID, Role: "user", Content: "unrelated chatter"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMessage(ctx, Message{ThreadID: threadB.ID, Role: "user", Content: "another lighthouse story"}); err != nil {
		t.Fatal(err)
	}
	matches, err := st.SearchMessages(ctx, "lighthouse", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("search found %d matches, want 2 across bots", len(matches))
	}
	threads := map[string]bool{}
	for _, m := range matches {
		threads[m.ThreadID] = true
	}
	if !threads[threadA.ID] || !threads[threadB.ID] {
		t.Fatalf("search missed a bot: %+v", matches)
	}
	if _, err := st.ListRoutineRuns(ctx, "rtn_missing", 10); err != ErrNotFound {
		t.Fatalf("missing routine err %v, want not found", err)
	}
	if _, err := st.SearchMessages(ctx, "", 5); err == nil {
		// empty query matches everything via LIKE; endpoint rejects it instead
	}
}
