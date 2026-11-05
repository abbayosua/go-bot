package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestExportImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	src, err := Open(filepath.Join(t.TempDir(), "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	bot, err := src.CreateBot(ctx, "B", "inst", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := src.CreateThread(ctx, bot.ID, "Chat")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.AddMessage(ctx, Message{ThreadID: thread.ID, Role: "user", Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	if _, err := src.CreateRoutine(ctx, bot.ID, thread.ID, "R", "prompt", "0 0 * * *", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := src.AddMemory(ctx, bot.ID, "fact"); err != nil {
		t.Fatal(err)
	}
	if err := src.SetSetting(ctx, "approval_mode", "auto"); err != nil {
		t.Fatal(err)
	}
	archive, err := src.ExportAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(archive)
	if err != nil {
		t.Fatal(err)
	}
	src.Close()
	dst, err := Open(filepath.Join(t.TempDir(), "dst.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if err := dst.ImportAll(ctx, raw); err != nil {
		t.Fatal(err)
	}
	bots, err := dst.ListBots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(bots) != 1 || bots[0].Name != "B" {
		t.Fatalf("bots %+v", bots)
	}
	messages, err := dst.ListMessages(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Content != "hello" {
		t.Fatalf("messages %+v", messages)
	}
	routines, err := dst.ListRoutines(ctx, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(routines) != 1 {
		t.Fatalf("routines %+v", routines)
	}
	if _, err := dst.ListAudit(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if err := dst.AddAudit(ctx, "test", "backup.import", "", ""); err != nil {
		t.Fatal(err)
	}
	rows, err := dst.ListAudit(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Action != "backup.import" {
		t.Fatalf("audit %+v", rows)
	}
	mode, err := dst.GetSetting(ctx, "approval_mode")
	if err != nil || mode != "auto" {
		t.Fatalf("setting %q err %v", mode, err)
	}
	stats, err := dst.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Queued != 0 {
		t.Fatalf("stats %+v", stats)
	}
}
