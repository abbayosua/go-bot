package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDMThreadIsShared(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a, err := st.CreateBot(ctx, "Ana", "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateBot(ctx, "Bo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.OpenDM(ctx, a.ID, a.ID); err == nil {
		t.Fatal("self DM allowed")
	}
	if _, _, err := st.OpenDM(ctx, a.ID, "bot_missing"); err == nil {
		t.Fatal("unknown peer allowed")
	}
	dm, thread, err := st.OpenDM(ctx, a.ID, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dm.MemberIDs) != 2 {
		t.Fatalf("dm %+v", dm)
	}
	// Reopening from either side returns the same thread.
	dm2, thread2, err := st.OpenDM(ctx, b.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dm2.ID != dm.ID || thread2.ID != thread.ID {
		t.Fatalf("reopen %+v %+v", dm2, thread2)
	}
	// Handoff header is the first message, readable by both.
	messages, err := st.ListMessages(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("header messages %+v", messages)
	}
	dms, err := st.ListDMs(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dms) != 1 || dms[0].ID != dm.ID {
		t.Fatalf("dms %+v", dms)
	}
	found, err := st.GetDMByThread(ctx, thread.ID)
	if err != nil || found == nil {
		t.Fatalf("by thread %+v err %v", found, err)
	}
	got, err := st.GetDM(ctx, dm.ID)
	if err != nil || got.ThreadID != thread.ID {
		t.Fatalf("by id %+v err %v", got, err)
	}
}
