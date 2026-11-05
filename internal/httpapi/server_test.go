package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"gobot/internal/agent"
	"gobot/internal/bus"
	"gobot/internal/config"
	"gobot/internal/llm"
	"gobot/internal/store"
)

func fakeModel(t *testing.T, tool string) *httptest.Server {
	t.Helper()
	var calls int
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		if calls == 1 && tool != "" {
			args, _ := json.Marshal(map[string]string{"path": "api.txt", "content": "written"})
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{
					"tool_calls": []any{map[string]any{
						"index": 0, "id": "call_1", "type": "function",
						"function": map[string]any{"name": tool, "arguments": string(args)},
					}},
				}}},
			}))
		} else {
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{"content": "All done."}}},
			}))
		}
		io.WriteString(w, "data: [DONE]\n\n")
	}))
}

func mustJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

type testServer struct {
	*httptest.Server
	store *store.Store
}

func newTestServer(t *testing.T, approval, tool string) *testServer {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	model := fakeModel(t, tool)
	t.Cleanup(model.Close)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := bus.New()
	engine := agent.NewEngine(st, llm.New(model.URL, ""), events, config.Provider{Model: "test"}, t.TempDir(), approval, log)
	server := &Server{Store: st, Engine: engine, Bus: events, Provider: config.Provider{Model: "test"}, Log: log}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return &testServer{Server: httpServer, store: st}
}

func (ts *testServer) do(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, ts.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var decoded map[string]any
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &decoded)
	}
	return resp.StatusCode, decoded
}

func TestSendMessageRunsTheAgent(t *testing.T) {
	ts := newTestServer(t, config.ApprovalNever, "write_file")

	status, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout"})
	if status != http.StatusCreated {
		t.Fatalf("create bot: %d %v", status, created)
	}
	thread := created["thread"].(map[string]any)
	threadID := thread["id"].(string)

	status, accepted := ts.do(t, http.MethodPost, "/api/threads/"+threadID+"/messages", map[string]string{"content": "write the file"})
	if status != http.StatusAccepted {
		t.Fatalf("post message: %d %v", status, accepted)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, threadBody := ts.do(t, http.MethodGet, "/api/threads/"+threadID, nil)
		messages := threadBody["messages"].([]any)
		last := messages[len(messages)-1].(map[string]any)
		if last["role"] == "assistant" && last["content"] == "All done." {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the run never produced a final assistant message")
}

func TestApprovalEndpointResumesTheRun(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "write_file")

	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout"})
	threadID := created["thread"].(map[string]any)["id"].(string)
	ts.do(t, http.MethodPost, "/api/threads/"+threadID+"/messages", map[string]string{"content": "write the file"})

	var approvalID string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && approvalID == "" {
		_, body := ts.do(t, http.MethodGet, "/api/threads/"+threadID, nil)
		for _, raw := range body["approvals"].([]any) {
			approval := raw.(map[string]any)
			if approval["status"] == store.ApprovalPending {
				approvalID = approval["id"].(string)
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if approvalID == "" {
		t.Fatal("no approval card appeared")
	}

	status, _ := ts.do(t, http.MethodPost, "/api/approvals/"+approvalID, map[string]string{"decision": "allow"})
	if status != http.StatusOK {
		t.Fatalf("decide approval: %d", status)
	}

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, body := ts.do(t, http.MethodGet, "/api/threads/"+threadID, nil)
		messages := body["messages"].([]any)
		for _, raw := range messages {
			message := raw.(map[string]any)
			if message["role"] == "assistant" && message["content"] == "All done." {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the resumed run never finished")
}

func TestSettingsAndValidation(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")

	status, body := ts.do(t, http.MethodPut, "/api/settings", map[string]string{"approval": "nope"})
	if status != http.StatusBadRequest {
		t.Errorf("invalid mode returned %d %v", status, body)
	}
	status, body = ts.do(t, http.MethodPut, "/api/settings", map[string]string{"approval": "never"})
	if status != http.StatusOK || body["approval"] != "never" {
		t.Errorf("set approval: %d %v", status, body)
	}
	status, _ = ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "  "})
	if status != http.StatusBadRequest {
		t.Errorf("empty bot name returned %d", status)
	}
	status, _ = ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Ok"})
	if status != http.StatusCreated {
		t.Errorf("create bot returned %d", status)
	}
	_, state := ts.do(t, http.MethodGet, "/api/state", nil)
	if state["approval"] != "never" {
		t.Errorf("state approval %v", state["approval"])
	}
}

func TestRoutineValidation(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout"})
	botID := created["bot"].(map[string]any)["id"].(string)

	status, _ := ts.do(t, http.MethodPost, "/api/bots/"+botID+"/routines", map[string]string{
		"prompt": "summarize", "cron": "not a cron",
	})
	if status != http.StatusBadRequest {
		t.Errorf("bad cron returned %d", status)
	}
	status, routine := ts.do(t, http.MethodPost, "/api/bots/"+botID+"/routines", map[string]string{
		"name": "Daily", "prompt": "summarize", "cron": "0 8 * * *",
	})
	if status != http.StatusCreated {
		t.Fatalf("create routine: %d %v", status, routine)
	}
	if routine["nextAt"].(float64) == 0 {
		t.Error("routine has no next run time")
	}
	status, _ = ts.do(t, http.MethodGet, "/api/bots/"+botID+"/routines", nil)
	if status != http.StatusOK {
		t.Errorf("list routines returned %d", status)
	}
}

func TestAuthTokenIsRequired(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	st := ts.store
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := bus.New()
	engine := agent.NewEngine(st, llm.New("http://127.0.0.1:1", ""), events, config.Provider{}, t.TempDir(), config.ApprovalAsk, log)
	guarded := httptest.NewServer((&Server{Store: st, Engine: engine, Bus: events, Log: log, Token: "secret"}).Handler())
	defer guarded.Close()

	resp, err := http.Get(guarded.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated request returned %d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, guarded.URL+"/api/state", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("authenticated request returned %d", resp.StatusCode)
	}
}

func TestMemoryAndSkillEndpoints(t *testing.T) {
	ts := newTestServer(t, config.ApprovalNever, "")
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout"})
	botID := created["bot"].(map[string]any)["id"].(string)

	ctx := context.Background()
	rec, err := ts.store.AddMemory(ctx, botID, "Deploy on Fridays.")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := ts.store.ListMemories(ctx, botID, 10)
	if err != nil || len(stored) != 1 || stored[0].Content != "Deploy on Fridays." {
		t.Fatalf("memories %+v err %v", stored, err)
	}
	status, _ := ts.do(t, http.MethodDelete, "/api/memories/"+rec.ID, nil)
	if status != http.StatusNoContent {
		t.Errorf("delete memory returned %d", status)
	}

	status, entry := ts.do(t, http.MethodPost, "/api/bots/"+botID+"/knowledge", map[string]string{
		"title": "Deploy", "body": "Migrate first.",
	})
	if status != http.StatusCreated {
		t.Fatalf("create knowledge: %d %v", status, entry)
	}
	status, _ = ts.do(t, http.MethodPost, "/api/bots/"+botID+"/knowledge", map[string]string{"title": "", "body": "x"})
	if status != http.StatusBadRequest {
		t.Errorf("empty knowledge title returned %d", status)
	}

	status, _ = ts.do(t, http.MethodPost, "/api/skills", map[string]string{
		"name": "Code Review", "description": "Review well", "body": "Check tests first.",
	})
	if status != http.StatusCreated {
		t.Fatalf("create skill returned %d", status)
	}
	got, err := ts.store.GetSkill(ctx, "code-review")
	if err != nil || got.Body != "Check tests first." {
		t.Fatalf("skill in store %+v err %v", got, err)
	}
	status, _ = ts.do(t, http.MethodDelete, "/api/skills/"+got.ID, nil)
	if status != http.StatusNoContent {
		t.Errorf("delete skill returned %d", status)
	}
}

func TestInstructionsProposeApplyUndo(t *testing.T) {
	ts := newTestServer(t, config.ApprovalNever, "")
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout", "instructions": "first"})
	botID := created["bot"].(map[string]any)["id"].(string)

	status, _ := ts.do(t, http.MethodPost, "/api/bots/"+botID+"/instructions", map[string]string{"instructions": "  "})
	if status != http.StatusBadRequest {
		t.Errorf("empty instructions returned %d", status)
	}
	status, proposed := ts.do(t, http.MethodPost, "/api/bots/"+botID+"/instructions", map[string]string{
		"instructions": "second", "source": "chat",
	})
	if status != http.StatusCreated {
		t.Fatalf("propose returned %d %v", status, proposed)
	}
	versionID := proposed["id"].(string)

	status, listed := ts.do(t, http.MethodGet, "/api/bots/"+botID+"/instructions", nil)
	if status != http.StatusOK {
		t.Fatalf("list returned %d", status)
	}
	if listed["instructions"] != "first" {
		t.Errorf("active instructions %v, want first", listed)
	}

	status, _ = ts.do(t, http.MethodPost, "/api/instructions/"+versionID+"/apply", nil)
	if status != http.StatusOK {
		t.Fatalf("apply returned %d", status)
	}
	status, listed = ts.do(t, http.MethodGet, "/api/bots/"+botID+"/instructions", nil)
	if status != http.StatusOK || listed["instructions"] != "second" {
		t.Errorf("after apply: %d %v", status, listed)
	}

	status, undone := ts.do(t, http.MethodPost, "/api/instructions/"+versionID+"/undo", nil)
	if status != http.StatusOK {
		t.Fatalf("undo returned %d", status)
	}
	if undone["instructions"] != "first" {
		t.Errorf("after undo instructions %v, want first", undone)
	}
	status, _ = ts.do(t, http.MethodPost, "/api/instructions/does-not-exist/apply", nil)
	if status != http.StatusNotFound {
		t.Errorf("missing version apply returned %d", status)
	}
}

func TestApprovalRulesCRUD(t *testing.T) {
	ts := newTestServer(t, config.ApprovalNever, "")
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout"})
	botID := created["bot"].(map[string]any)["id"].(string)

	status, _ := ts.do(t, http.MethodPost, "/api/rules", map[string]string{"kind": "maybe"})
	if status != http.StatusBadRequest {
		t.Errorf("bad kind returned %d", status)
	}
	status, rule := ts.do(t, http.MethodPost, "/api/rules", map[string]string{
		"scope": botID, "kind": "allow", "tool": "shell", "args": `{"command":"ls"}`,
	})
	if status != http.StatusCreated {
		t.Fatalf("create rule returned %d %v", status, rule)
	}
	ruleID := rule["id"].(string)

	status, _ = ts.do(t, http.MethodGet, "/api/rules?botId="+botID, nil)
	if status != http.StatusOK {
		t.Fatalf("list returned %d", status)
	}
	stored, err := ts.store.ListApprovalRules(context.Background(), botID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("store has %+v err %v", stored, err)
	}

	status, _ = ts.do(t, http.MethodDelete, "/api/rules/"+ruleID, nil)
	if status != http.StatusNoContent {
		t.Errorf("delete returned %d", status)
	}
	stored, err = ts.store.ListApprovalRules(context.Background(), botID)
	if err != nil || len(stored) != 0 {
		t.Errorf("rule was not deleted: %+v err %v", stored, err)
	}
}
