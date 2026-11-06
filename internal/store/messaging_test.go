package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMsgInstallShareList(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bot, err := st.CreateBot(ctx, "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := st.CreateThread(ctx, bot.ID, "Chat")
	if err != nil {
		t.Fatal(err)
	}
	install, err := st.AddMsgInstall(ctx, bot.ID, "telegram", "tok-123", "111, 222,111", thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if install.Token != "" {
		t.Fatal("token leaked in create response")
	}
	if install.Chats != "111,222" {
		t.Fatalf("chats %q", install.Chats)
	}
	if !ChatAllowed(install.Chats, "111") || ChatAllowed(install.Chats, "999") || ChatAllowed("", "111") {
		t.Fatal("allowlist wrong")
	}
	found, raw, err := st.MsgInstallForToken(ctx, "tok-123")
	if err != nil || found.ID != install.ID || raw != "tok-123" {
		t.Fatalf("token lookup %+v %q err %v", found, raw, err)
	}
	if err := st.SetMsgChats(ctx, install.ID, "333"); err != nil {
		t.Fatal(err)
	}
	updated, _, err := st.MsgInstallForToken(ctx, "tok-123")
	if err != nil || updated.Chats != "333" {
		t.Fatalf("chats %+v err %v", updated, err)
	}
	lists, err := st.ListMsgInstalls(ctx, bot.ID)
	if err != nil || len(lists) != 1 {
		t.Fatalf("list %+v err %v", lists, err)
	}
	if err := st.DeleteMsgInstall(ctx, install.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.MsgInstallForToken(ctx, "tok-123"); err == nil {
		t.Fatal("deleted install resolves")
	}
}
