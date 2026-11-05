package store

import (
	"context"
	"testing"
)

func TestArtifactsListWriteFiles(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	bot, err := st.CreateBot(ctx, "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := st.CreateThread(ctx, bot.ID, "Chat")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMessage(ctx, Message{ThreadID: thread.ID, Role: "tool", Content: "wrote 11 bytes to notes/hello.txt", ToolCallID: "c1", ToolName: "write_file"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMessage(ctx, Message{ThreadID: thread.ID, Role: "tool", Content: "listed stuff", ToolCallID: "c2", ToolName: "list_files"}); err != nil {
		t.Fatal(err)
	}
	artifacts, err := st.ListArtifacts(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Path != "notes/hello.txt" || artifacts[0].Size != 11 {
		t.Fatalf("artifacts %+v", artifacts)
	}
}
