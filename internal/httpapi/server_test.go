package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
	store  *store.Store
	engine *agent.Engine
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
	return &testServer{Server: httpServer, store: st, engine: engine}
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

func TestShareLinksAndAgentImport(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	ctx := context.Background()
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout"})
	botID := created["bot"].(map[string]any)["id"].(string)
	threads, err := ts.store.ListThreads(ctx, botID)
	if err != nil || len(threads) == 0 {
		t.Fatalf("threads %+v err %v", threads, err)
	}
	threadID := threads[0].ID
	status, share := ts.do(t, http.MethodPost, "/api/threads/"+threadID+"/shares", nil)
	if status != http.StatusCreated {
		t.Fatalf("create share: %d", status)
	}
	token, _ := share["token"].(string)
	if token == "" {
		t.Fatal("share token empty")
	}
	resp, err := http.Get(ts.URL + "/api/shared/" + token)
	if err != nil {
		t.Fatal(err)
	}
	var shared map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&shared); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("shared thread: %d", resp.StatusCode)
	}
	shareID, _ := share["id"].(string)
	status, _ = ts.do(t, http.MethodDelete, "/api/shares/"+shareID, nil)
	if status != http.StatusNoContent {
		t.Errorf("revoke share: %d", status)
	}
	revoked, err := http.Get(ts.URL + "/api/shared/" + token)
	if err != nil {
		t.Fatal(err)
	}
	revoked.Body.Close()
	if revoked.StatusCode != http.StatusNotFound {
		t.Errorf("revoked link: %d, want 404", revoked.StatusCode)
	}
}

func TestScratchpadEndpoints(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout"})
	botID := created["bot"].(map[string]any)["id"].(string)
	status, _ := ts.do(t, http.MethodPost, "/api/bots/"+botID+"/scratchpad", map[string]string{"text": ""})
	if status != http.StatusBadRequest {
		t.Errorf("empty note returned %d, want 400", status)
	}
	status, _ = ts.do(t, http.MethodPost, "/api/bots/"+botID+"/scratchpad", map[string]string{"text": "check the logs"})
	if status != http.StatusCreated {
		t.Fatalf("add note: %d", status)
	}
	resp, err := http.Get(ts.URL + "/api/bots/" + botID + "/scratchpad")
	if err != nil {
		t.Fatal(err)
	}
	var notes []string
	if err := json.NewDecoder(resp.Body).Decode(&notes); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(notes) != 1 {
		t.Fatalf("notes %v", notes)
	}
	status, _ = ts.do(t, http.MethodDelete, "/api/bots/"+botID+"/scratchpad", nil)
	if status != http.StatusOK {
		t.Errorf("clear notes: %d", status)
	}
	empty, err := http.Get(ts.URL + "/api/bots/" + botID + "/scratchpad")
	if err != nil {
		t.Fatal(err)
	}
	var cleared []string
	if err := json.NewDecoder(empty.Body).Decode(&cleared); err != nil {
		t.Fatal(err)
	}
	empty.Body.Close()
	if len(cleared) != 0 {
		t.Fatalf("notes after clear %v", cleared)
	}
}

func TestAttachmentUploadDownload(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	ctx := context.Background()
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout"})
	botID := created["bot"].(map[string]any)["id"].(string)
	threads, err := ts.store.ListThreads(ctx, botID)
	if err != nil || len(threads) == 0 {
		t.Fatalf("threads %+v err %v", threads, err)
	}
	threadID := threads[0].ID
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "hello attachment"); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/threads/"+threadID+"/attachments", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var uploaded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&uploaded); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d %v", resp.StatusCode, uploaded)
	}
	attachmentID, _ := uploaded["id"].(string)
	download, err := http.Get(ts.URL + "/api/attachments/" + attachmentID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(download.Body)
	download.Body.Close()
	if string(raw) != "hello attachment" {
		t.Fatalf("download %q", raw)
	}
	listed, err := http.Get(ts.URL + "/api/threads/" + threadID + "/attachments")
	if err != nil {
		t.Fatal(err)
	}
	var items []map[string]any
	if err := json.NewDecoder(listed.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	listed.Body.Close()
	if len(items) != 1 {
		t.Fatalf("list %+v", items)
	}
}

func TestSpacesCRUDAndFilter(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout"})
	if created["bot"] == nil {
		t.Fatalf("create bot: %v", created)
	}
	status, space := ts.do(t, http.MethodPost, "/api/spaces", map[string]string{"name": "Work"})
	if status != http.StatusCreated {
		t.Fatalf("create space: %d", status)
	}
	spaceID := space["id"].(string)
	status, _ = ts.do(t, http.MethodPost, "/api/spaces", map[string]string{"name": ""})
	if status != http.StatusBadRequest {
		t.Errorf("empty space returned %d", status)
	}
	status, _ = ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Worker", "spaceId": spaceID})
	if status != http.StatusCreated {
		t.Fatalf("create bot in space: %d", status)
	}
	resp, err := http.Get(ts.URL + "/api/state?spaceId=" + spaceID)
	if err != nil {
		t.Fatal(err)
	}
	var filtered map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&filtered); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	bots := filtered["bots"].([]any)
	if len(bots) != 1 {
		t.Fatalf("filtered bots %d, want 1", len(bots))
	}
	if _, ok := filtered["spaces"]; !ok {
		t.Fatal("state misses spaces")
	}
	// Non-empty space refuses deletion.
	status, _ = ts.do(t, http.MethodDelete, "/api/spaces/"+spaceID, nil)
	if status != http.StatusBadRequest {
		t.Errorf("delete non-empty space: %d, want 400", status)
	}
	// Unknown space id yields no bots, not an error.
	empty, err := http.Get(ts.URL + "/api/state?spaceId=spc_missing")
	if err != nil {
		t.Fatal(err)
	}
	var missing map[string]any
	if err := json.NewDecoder(empty.Body).Decode(&missing); err != nil {
		t.Fatal(err)
	}
	empty.Body.Close()
	if len(missing["bots"].([]any)) != 0 {
		t.Fatalf("missing space bots %+v", missing["bots"])
	}
}

func TestFleetSwitchEndpoints(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout"})
	botID := created["bot"].(map[string]any)["id"].(string)
	status, _ := ts.do(t, http.MethodPut, "/api/bots/"+botID+"/fleet", map[string]bool{"fleet": true})
	if status != http.StatusOK {
		t.Fatalf("fleet on: %d", status)
	}
	resp, err := http.Get(ts.URL + "/api/bots/" + botID + "/fleet")
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !state["fleet"] {
		t.Fatal("fleet switch did not stick")
	}
}

func TestMCPServersRejectBadProbe(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	status, _ := ts.do(t, http.MethodPost, "/api/mcp/servers", map[string]string{"name": "Down", "url": "http://127.0.0.1:1"})
	if status != http.StatusBadGateway {
		t.Errorf("down server: %d, want 502", status)
	}
	status, _ = ts.do(t, http.MethodPost, "/api/mcp/servers", map[string]string{"name": "", "url": "http://127.0.0.1:1"})
	if status != http.StatusBadRequest {
		t.Errorf("nameless server: %d, want 400", status)
	}
}

func TestMessagingInboundAllowlist(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout"})
	botID := created["bot"].(map[string]any)["id"].(string)
	status, _ := ts.do(t, http.MethodPost, "/api/bots/"+botID+"/msg", map[string]string{"token": "", "chats": "111"})
	if status != http.StatusBadRequest {
		t.Errorf("empty token: %d, want 400", status)
	}
	status, install := ts.do(t, http.MethodPost, "/api/bots/"+botID+"/msg",
		map[string]string{"adapter": "telegram", "token": "tok-abc", "chats": "111"})
	if status != http.StatusCreated {
		t.Fatalf("connect: %d %v", status, install)
	}
	installID := install["id"].(string)
	if _, leaked := install["token"]; leaked {
		t.Fatal("token leaked in create response")
	}
	// Unknown chat dropped before any run.
	dropped, err := http.Post(ts.URL+"/api/msg/in/tok-abc", "application/json",
		strings.NewReader(`{"update_id":1,"message":{"message_id":1,"chat":{"id":999},"from":{"id":1,"username":"mallory"},"text":"hi"}}`))
	if err != nil {
		t.Fatal(err)
	}
	var drop map[string]string
	_ = json.NewDecoder(dropped.Body).Decode(&drop)
	dropped.Body.Close()
	if drop["status"] != "dropped" {
		t.Fatalf("unknown chat %+v", drop)
	}
	// Allowlisted chat starts a run through the approval gate.
	accepted, err := http.Post(ts.URL+"/api/msg/in/tok-abc", "application/json",
		strings.NewReader(`{"update_id":2,"message":{"message_id":2,"chat":{"id":111},"from":{"id":2,"username":"ana"},"text":"hello"}}`))
	if err != nil {
		t.Fatal(err)
	}
	var inbound map[string]string
	_ = json.NewDecoder(accepted.Body).Decode(&inbound)
	accepted.Body.Close()
	if accepted.StatusCode != http.StatusAccepted || inbound["runId"] == "" {
		t.Fatalf("inbound %+v code %d", inbound, accepted.StatusCode)
	}
	// Removing the chat stops its messages.
	status, _ = ts.do(t, http.MethodPut, "/api/msg/"+installID, map[string]string{"chats": "222"})
	if status != http.StatusOK {
		t.Fatalf("share update: %d", status)
	}
	revoked, err := http.Post(ts.URL+"/api/msg/in/tok-abc", "application/json",
		strings.NewReader(`{"update_id":3,"message":{"message_id":3,"chat":{"id":111},"from":{"id":2,"username":"ana"},"text":"again"}}`))
	if err != nil {
		t.Fatal(err)
	}
	var gone map[string]string
	_ = json.NewDecoder(revoked.Body).Decode(&gone)
	revoked.Body.Close()
	if gone["status"] != "dropped" {
		t.Fatalf("removed chat %+v", gone)
	}
	// Unknown install 404s.
	missing, err := http.Post(ts.URL+"/api/msg/in/tok-nope", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Errorf("missing install: %d, want 404", missing.StatusCode)
	}
}

func TestXAgentsRegisterAndCallback(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	status, _ := ts.do(t, http.MethodPost, "/api/xagents", map[string]string{"name": "Out", "url": "not-a-url"})
	if status != http.StatusBadRequest {
		t.Errorf("bad url: %d, want 400", status)
	}
	status, agent := ts.do(t, http.MethodPost, "/api/xagents", map[string]string{"name": "Out", "url": "http://127.0.0.1:1/"})
	if status != http.StatusCreated {
		t.Fatalf("register: %d %v", status, agent)
	}
	agentID := agent["id"].(string)
	ctx := context.Background()
	task, err := ts.store.CreateExternalTask(ctx, agentID, "run_1", "do research")
	if err != nil {
		t.Fatal(err)
	}
	if task.Token == "" || task.Status != "pending" {
		t.Fatalf("task %+v", task)
	}
	status, done := ts.do(t, http.MethodPost, "/api/xagents/in/"+task.Token, map[string]string{"token": task.Token, "result": "done: 42"})
	if status != http.StatusOK {
		t.Fatalf("callback: %d %v", status, done)
	}
	stored, err := ts.store.ExternalTaskResult(ctx, "run_1", task.ID)
	if err != nil || stored.Result != "done: 42" || stored.Status != "done" {
		t.Fatalf("result %+v err %v", stored, err)
	}
	status, _ = ts.do(t, http.MethodPost, "/api/xagents/in/tok_missing", map[string]string{"token": "tok_missing", "result": "x"})
	if status != http.StatusNotFound {
		t.Errorf("missing token: %d, want 404", status)
	}
	status, _ = ts.do(t, http.MethodDelete, "/api/xagents/"+agentID, nil)
	if status != http.StatusNoContent {
		t.Errorf("delete: %d", status)
	}
}

func TestMultiUserFlow(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	status, first := ts.do(t, http.MethodPost, "/api/users", map[string]string{"name": "owner", "password": "correct horse 1!"})
	if status != http.StatusCreated {
		t.Fatalf("register owner: %d %v", status, first)
	}
	if first["role"] != "owner" {
		t.Fatalf("first role %v", first)
	}
	status, _ = ts.do(t, http.MethodPost, "/api/users", map[string]string{"name": "viewer", "password": "another pass 2@"})
	if status != http.StatusCreated {
		t.Fatalf("register viewer: %d", status)
	}
	status, _ = ts.do(t, http.MethodPost, "/api/session", map[string]string{"name": "owner", "password": "nope, wrong 1!"})
	if status != http.StatusUnauthorized {
		t.Errorf("bad login: %d, want 401", status)
	}
	status, login := ts.do(t, http.MethodPost, "/api/session", map[string]string{"name": "owner", "password": "correct horse 1!"})
	if status != http.StatusOK {
		t.Fatalf("login: %d %v", status, login)
	}
	token := login["token"].(string)
	authGet := func(path string) (int, map[string]any) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	status, me := authGet("/api/me")
	if status != http.StatusOK {
		t.Fatalf("me: %d %v", status, me)
	}
	if me["user"].(map[string]any)["name"] != "owner" {
		t.Fatalf("me %+v", me)
	}
	// Viewer token cannot list users (owners only).
	status, vlogin := ts.do(t, http.MethodPost, "/api/session", map[string]string{"name": "viewer", "password": "another pass 2@"})
	if status != http.StatusOK {
		t.Fatalf("viewer login: %d", status)
	}
	vreq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/users", nil)
	vreq.Header.Set("Authorization", "Bearer "+vlogin["token"].(string))
	vresp, err := http.DefaultClient.Do(vreq)
	if err != nil {
		t.Fatal(err)
	}
	vresp.Body.Close()
	if vresp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer list users: %d, want 403", vresp.StatusCode)
	}
	// Logout kills the token.
	lreq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/session", nil)
	lreq.Header.Set("Authorization", "Bearer "+token)
	lresp, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatal(err)
	}
	lresp.Body.Close()
	if lresp.StatusCode != http.StatusOK {
		t.Errorf("logout: %d", lresp.StatusCode)
	}
	status, _ = authGet("/api/me")
	if status != http.StatusUnauthorized {
		t.Errorf("me after logout: %d, want 401", status)
	}
}

func TestWalletTopUpAndCaps(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout"})
	botID := created["bot"].(map[string]any)["id"].(string)
	status, _ := ts.do(t, http.MethodPost, "/api/bots/"+botID+"/wallet/topup", map[string]any{"amount": 0})
	if status != http.StatusBadRequest {
		t.Errorf("zero topup: %d, want 400", status)
	}
	status, topped := ts.do(t, http.MethodPost, "/api/bots/"+botID+"/wallet/topup", map[string]any{"amount": 50, "note": "grant"})
	if status != http.StatusOK {
		t.Fatalf("topup: %d %v", status, topped)
	}
	if err := ts.store.SetSetting(context.Background(), "wallet_cap:call", "10"); err != nil {
		t.Fatal(err)
	}
	status, _ = ts.do(t, http.MethodPost, "/api/bots/"+botID+"/wallet/charge", map[string]any{"action": "call", "amount": 30})
	if status != http.StatusBadRequest {
		t.Errorf("over-cap: %d, want 400", status)
	}
	status, receipt := ts.do(t, http.MethodPost, "/api/bots/"+botID+"/wallet/charge", map[string]any{"action": "call", "amount": 6})
	if status != http.StatusOK {
		t.Fatalf("charge: %d %v", status, receipt)
	}
	resp, err := http.Get(ts.URL + "/api/bots/" + botID + "/wallet")
	if err != nil {
		t.Fatal(err)
	}
	var wallet map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&wallet)
	resp.Body.Close()
	if wallet["balance"].(float64) != 44 {
		t.Fatalf("wallet %+v", wallet)
	}
}

func TestAppConnectionsRefreshNoUserAction(t *testing.T) {
	ts := newTestServer(t, config.ApprovalAsk, "")
	_, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Scout"})
	botID := created["bot"].(map[string]any)["id"].(string)
	var refreshed string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != "refresh_token" || body["refresh_token"] != "refresh-1" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		refreshed = "access-2"
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"access-2","refresh_token":"refresh-2"}`)
	}))
	defer tokenServer.Close()
	status, _ := ts.do(t, http.MethodPost, "/api/bots/"+botID+"/apps",
		map[string]string{"name": "gh", "provider": "github", "access": "access-1", "refresh": "refresh-1", "tokenUrl": tokenServer.URL})
	if status != http.StatusCreated {
		t.Fatalf("connect: %d", status)
	}
	if got, err := ts.store.AppAccessToken(context.Background(), "gh"); err != nil || got != "access-1" {
		t.Fatalf("vault %q err %v", got, err)
	}
	resp, err := http.Get(ts.URL + "/api/bots/" + botID + "/apps")
	if err != nil {
		t.Fatal(err)
	}
	var conns []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&conns)
	resp.Body.Close()
	if len(conns) != 1 || conns[0]["access"] != nil {
		t.Fatalf("list leaks: %+v", conns)
	}
	status, _ = ts.do(t, http.MethodPost, "/api/apps/gh/refresh", map[string]string{})
	if status != http.StatusOK {
		t.Fatalf("refresh: %d", status)
	}
	if refreshed != "access-2" {
		t.Fatal("refresh endpoint not hit")
	}
	if got, err := ts.store.AppAccessToken(context.Background(), "gh"); err != nil || got != "access-2" {
		t.Fatalf("rotated %q err %v", got, err)
	}
	status, _ = ts.do(t, http.MethodDelete, "/api/apps/gh", nil)
	if status != http.StatusNoContent {
		t.Errorf("disconnect: %d", status)
	}
	if _, err := ts.store.AppAccessToken(context.Background(), "gh"); err == nil {
		t.Fatal("token survives disconnect")
	}
}
