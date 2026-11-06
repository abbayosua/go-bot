package httpapi

import (
	"net/http"
	"testing"

	"gobot/internal/config"
)

func TestMentionRouting(t *testing.T) {
	names := map[string]string{"bot_a": "Ana S", "bot_b": "Bo"}
	if got := mentionMembers("hi @ana, thoughts?", names); len(got) != 1 || got[0] != "bot_a" {
		t.Fatalf("mention %+v", got)
	}
	if got := mentionMembers("hi @BO", names); len(got) != 1 || got[0] != "bot_b" {
		t.Fatalf("case mention %+v", got)
	}
	if got := mentionMembers("hello everyone", names); len(got) != 0 {
		t.Fatalf("bare message should route to everyone, got %+v", got)
	}
	if got := mentionMembers("hi @ghost", names); len(got) != 0 {
		t.Fatalf("unknown mention %+v", got)
	}
	if got := mentionMembers("@ana and @bo go", names); len(got) != 2 {
		t.Fatalf("double mention %+v", got)
	}
}

func TestGroupPostFansOut(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Ana"})
	aID := created["bot"].(map[string]any)["id"].(string)
	_, created = ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Bo"})
	bID := created["bot"].(map[string]any)["id"].(string)
	status, group := ts.do(t, http.MethodPost, "/api/groups", map[string]any{
		"members": []string{aID, bID}, "title": "Duo",
	})
	if status != http.StatusCreated {
		t.Fatalf("create group: %d %v", status, group)
	}
	threadID := group["thread"].(map[string]any)["id"].(string)
	// Mention routes to one member only.
	status, posted := ts.do(t, http.MethodPost, "/api/threads/"+threadID+"/messages",
		map[string]string{"content": "hi @ana, your turn"})
	if status != http.StatusAccepted {
		t.Fatalf("mention post: %d %v", status, posted)
	}
	runs := posted["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("mention runs %d, want 1", len(runs))
	}
	// Bare message asks everyone.
	status, posted = ts.do(t, http.MethodPost, "/api/threads/"+threadID+"/messages",
		map[string]string{"content": "hello everyone"})
	if status != http.StatusAccepted {
		t.Fatalf("bare post: %d %v", status, posted)
	}
	if len(posted["runs"].([]any)) != 2 {
		t.Fatalf("bare runs %+v", posted["runs"])
	}
	// Bounds enforced at the API.
	status, _ = ts.do(t, http.MethodPost, "/api/groups", map[string]any{
		"members": []string{aID}, "title": "Solo",
	})
	if status != http.StatusBadRequest {
		t.Errorf("solo group: %d, want 400", status)
	}
}
