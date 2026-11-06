package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttachmentEncryptsAtRest(t *testing.T) {
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
	secret := []byte("the launch codes are 12345")
	attachment, err := st.SaveAttachment(ctx, thread.ID, "session-abc", "codes.txt", "text/plain", secret)
	if err != nil {
		t.Fatal(err)
	}
	// The raw database must not reveal the content.
	var sealed []byte
	if err := st.db.QueryRowContext(ctx, `SELECT sealed FROM attachment_blobs WHERE id = ?`, attachment.ID).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), "launch codes") {
		t.Fatal("plaintext visible in blob")
	}
	var count int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM attachment_blobs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("blobs %d", count)
	}
	// The uploading session reads it back intact.
	opened, content, err := st.OpenAttachment(ctx, attachment.ID, "session-abc")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(secret) || opened.Filename != "codes.txt" {
		t.Fatalf("round trip %+v %q", opened, content)
	}
	if opened.SessionID != "" {
		t.Fatal("session leaked in metadata response")
	}
	// Another session gets nothing.
	if _, _, err := st.OpenAttachment(ctx, attachment.ID, "session-xyz"); err == nil {
		t.Fatal("foreign session opened the blob")
	}
	if _, _, err := st.OpenAttachment(ctx, attachment.ID, ""); err == nil {
		t.Fatal("empty session opened the blob")
	}
	listed, err := st.ListAttachments(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != attachment.ID {
		t.Fatalf("list %+v", listed)
	}
	if _, err := st.SaveAttachment(ctx, thread.ID, "", "x.txt", "text/plain", []byte("x")); err == nil {
		t.Fatal("empty session upload allowed")
	}
}
