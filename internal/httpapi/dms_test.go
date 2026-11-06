package httpapi

import (
	"context"
	"net/http"
	"testing"

	"gobot/internal/config"
)

func TestDMFlow(t *testing.T) {
	ts := newTestServer(t, config.ApprovalNever, "")
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Ana"})
	aID := created["bot"].(map[string]any)["id"].(string)
	_, created = ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Bo"})
	bID := created["bot"].(map[string]any)["id"].(string)
	// Self DM refused.
	status, _ := ts.do(t, http.MethodPost, "/api/bots/"+aID+"/dms", map[string]string{"with": aID})
	if status != http.StatusBadRequest {
		t.Errorf("self DM: %d, want 400", status)
	}
	status, opened := ts.do(t, http.MethodPost, "/api/bots/"+aID+"/dms", map[string]string{"with": "Bo"})
	if status != http.StatusOK {
		t.Fatalf("open DM: %d %v", status, opened)
	}
	dmID := opened["dm"].(map[string]any)["id"].(string)
	threadID := opened["thread"].(map[string]any)["id"].(string)
	// Non-member cannot post.
	status, _ = ts.do(t, http.MethodPost, "/api/dms/"+dmID+"/messages",
		map[string]string{"from": "bot_ghost", "content": "hi"})
	if status != http.StatusBadRequest {
		t.Errorf("ghost post: %d, want 400", status)
	}
	// Ana writes; the run belongs to Bo, the peer.
	status, posted := ts.do(t, http.MethodPost, "/api/dms/"+dmID+"/messages",
		map[string]string{"from": aID, "content": "Bo, what is the deploy port?"})
	if status != http.StatusAccepted {
		t.Fatalf("send DM: %d %v", status, posted)
	}
	if posted["run"].(map[string]any)["botId"] != bID {
		t.Fatalf("run not on peer: %v", posted["run"])
	}
	message := posted["message"].(map[string]any)
	if message["content"] != "Ana: Bo, what is the deploy port?" {
		t.Fatalf("handoff %q misses the sender name", message["content"])
	}
	// Cross-citation: Bo's history shows Ana's words prefixed with her name.
	ctx := context.Background()
	messages, err := ts.store.ListMessages(ctx, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].Content != "Ana: Bo, what is the deploy port?" {
		t.Fatalf("history %+v", messages)
	}
}
