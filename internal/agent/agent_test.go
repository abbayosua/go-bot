package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gobot/internal/bus"
	"gobot/internal/config"
	"gobot/internal/llm"
	"gobot/internal/memory"
	"gobot/internal/store"
)

// fakeProvider replays one scripted SSE response per request.
type fakeProvider struct {
	mu     sync.Mutex
	calls  int
	script [][]string
}

func (f *fakeProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	f.mu.Lock()
	index := f.calls
	f.calls++
	f.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	payloads := []string{contentChunk("done")}
	if index < len(f.script) {
		payloads = f.script[index]
	}
	for _, payload := range payloads {
		fmt.Fprintf(w, "data: %s\n\n", payload)
	}
	io.WriteString(w, "data: [DONE]\n\n")
}

func (f *fakeProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func contentChunk(text string) string {
	raw, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]any{"content": text}}},
	})
	return string(raw)
}

func toolChunk(id, name string, args any) string {
	encoded, _ := json.Marshal(args)
	raw, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"delta": map[string]any{
				"tool_calls": []any{map[string]any{
					"index": 0,
					"id":    id,
					"type":  "function",
					"function": map[string]any{
						"name":      name,
						"arguments": string(encoded),
					},
				}},
			},
		}},
	})
	return string(raw)
}

type harness struct {
	store  *store.Store
	engine *Engine
	bank   *fakeProvider
	bot    store.Bot
	thread store.Thread
}

func newHarness(t *testing.T, mode string, script [][]string) *harness {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	bank := &fakeProvider{script: script}
	server := httptest.NewServer(bank)
	t.Cleanup(server.Close)

	ctx := context.Background()
	bot, err := st.CreateBot(ctx, "Tester", "Be brief.", "")
	if err != nil {
		t.Fatalf("create bot: %v", err)
	}
	thread, err := st.CreateThread(ctx, bot.ID, "Chat")
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := llm.New(server.URL, "")
	engine := NewEngine(st, client, bus.New(), config.Provider{Model: "test-model"}, t.TempDir(), mode, log)
	return &harness{store: st, engine: engine, bank: bank, bot: bot, thread: thread}
}

func (h *harness) startRun(t *testing.T, prompt string) store.Run {
	t.Helper()
	ctx := context.Background()
	if _, err := h.store.AddMessage(ctx, store.Message{ThreadID: h.thread.ID, Role: "user", Content: prompt}); err != nil {
		t.Fatalf("add message: %v", err)
	}
	run, err := h.store.CreateRun(ctx, h.bot.ID, h.thread.ID, "user", prompt)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := h.engine.Execute(ctx, run.ID); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return run
}

func (h *harness) messages(t *testing.T) []store.Message {
	t.Helper()
	messages, err := h.store.ListMessages(context.Background(), h.thread.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	return messages
}

func TestRunExecutesToolThenFinishes(t *testing.T) {
	h := newHarness(t, config.ApprovalNever, [][]string{
		{toolChunk("call_1", "write_file", map[string]string{"path": "out.txt", "content": "hello"})},
		{contentChunk("Wrote out.txt.")},
	})
	run := h.startRun(t, "save hello to out.txt")

	stored, err := h.store.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != store.RunDone {
		t.Fatalf("status %q, want done (%s)", stored.Status, stored.Error)
	}
	if _, err := os.Stat(filepath.Join(h.engine.Workspaces, h.bot.ID, "out.txt")); err != nil {
		t.Errorf("tool did not create the file: %v", err)
	}
	var roles []string
	for _, msg := range h.messages(t) {
		roles = append(roles, msg.Role)
	}
	want := []string{"user", "assistant", "tool", "assistant"}
	if strings.Join(roles, ",") != strings.Join(want, ",") {
		t.Errorf("message roles %v, want %v", roles, want)
	}
	if h.bank.callCount() != 2 {
		t.Errorf("provider called %d times, want 2", h.bank.callCount())
	}
}

func TestAskModePausesThenResumes(t *testing.T) {
	h := newHarness(t, config.ApprovalAsk, [][]string{
		{toolChunk("call_1", "shell", map[string]string{"command": "echo hi"})},
		{contentChunk("Ran the command.")},
	})
	run := h.startRun(t, "run echo hi")

	paused, err := h.store.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paused.Status != store.RunWaitingApproval {
		t.Fatalf("status %q, want waiting_approval", paused.Status)
	}
	approvals, err := h.store.ListApprovals(context.Background(), h.thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(approvals) != 1 || approvals[0].Tool != "shell" {
		t.Fatalf("approvals %+v, want one shell approval", approvals)
	}
	if !strings.Contains(approvals[0].Args, "echo hi") {
		t.Errorf("approval args %q do not show the command", approvals[0].Args)
	}

	if _, err := h.store.DecideApproval(context.Background(), approvals[0].ID, store.ApprovalAllow); err != nil {
		t.Fatal(err)
	}
	if err := h.engine.Execute(context.Background(), run.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}

	finished, err := h.store.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != store.RunDone {
		t.Fatalf("status %q after resume, want done", finished.Status)
	}
	var toolOutput string
	for _, msg := range h.messages(t) {
		if msg.Role == "tool" {
			toolOutput = msg.Content
		}
	}
	if !strings.Contains(toolOutput, "hi") {
		t.Errorf("tool output %q does not contain the command result", toolOutput)
	}
}

func TestDeniedToolIsReportedAndRunContinues(t *testing.T) {
	h := newHarness(t, config.ApprovalAsk, [][]string{
		{toolChunk("call_1", "shell", map[string]string{"command": "echo nope"})},
		{contentChunk("Understood.")},
	})
	run := h.startRun(t, "run echo nope")
	approvals, err := h.store.ListApprovals(context.Background(), h.thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(approvals) != 1 {
		t.Fatalf("got %d approvals, want 1", len(approvals))
	}
	if _, err := h.store.DecideApproval(context.Background(), approvals[0].ID, store.ApprovalDeny); err != nil {
		t.Fatal(err)
	}
	if err := h.engine.Execute(context.Background(), run.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	finished, err := h.store.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != store.RunDone {
		t.Fatalf("status %q, want done", finished.Status)
	}
	var denied bool
	for _, msg := range h.messages(t) {
		if msg.Role == "tool" && strings.Contains(msg.Content, "denied") {
			denied = true
		}
	}
	if !denied {
		t.Error("the model was not told the tool was denied")
	}
}

func TestLoopGuardStopsRepeats(t *testing.T) {
	repeat := toolChunk("call_same", "list_files", map[string]string{"path": ""})
	script := make([][]string, loopGuardLimit+2)
	for i := range script {
		script[i] = []string{repeat}
	}
	h := newHarness(t, config.ApprovalNever, script)
	run := h.startRun(t, "list the files")

	finished, err := h.store.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != store.RunDone {
		t.Fatalf("status %q, want done", finished.Status)
	}
	var last string
	for _, msg := range h.messages(t) {
		if msg.Role == "assistant" {
			last = msg.Content
		}
	}
	if !strings.Contains(last, "got stuck") {
		t.Errorf("final assistant message %q does not explain the stop", last)
	}
}

func TestCancelledRunStops(t *testing.T) {
	h := newHarness(t, config.ApprovalNever, [][]string{{contentChunk("hi")}})
	ctx := context.Background()
	run, err := h.store.CreateRun(ctx, h.bot.ID, h.thread.ID, "user", "say hi")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.UpdateRun(ctx, run.ID, store.RunCancelled, "", 0); err != nil {
		t.Fatal(err)
	}
	if err := h.engine.Execute(ctx, run.ID); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if h.bank.callCount() != 0 {
		t.Errorf("provider was called %d times for a cancelled run", h.bank.callCount())
	}
}

func TestMissingToolCallsFindsUnansweredCall(t *testing.T) {
	history := []store.Message{
		{Role: "user", Content: "do it"},
		{Role: "assistant", ToolCalls: marshalCalls([]llm.ToolCall{{ID: "a", Function: llm.FunctionCall{Name: "shell", Arguments: "{}"}}})},
		{Role: "tool", ToolCallID: "a", Content: "ok"},
		{Role: "assistant", Content: "done"},
	}
	if pending := missingToolCalls(history); len(pending) != 0 {
		t.Errorf("got %d pending calls, want 0", len(pending))
	}
	history = append(history, store.Message{
		Role:      "assistant",
		ToolCalls: marshalCalls([]llm.ToolCall{{ID: "b", Function: llm.FunctionCall{Name: "shell", Arguments: "{}"}}}),
	})
	pending := missingToolCalls(history)
	if len(pending) != 1 || pending[0].ID != "b" {
		t.Errorf("pending %+v, want the unanswered call b", pending)
	}
}

func TestRememberAndRecallAcrossRuns(t *testing.T) {
	scripted := newScriptedServer(t, [][]string{
		{toolChunk("call_rem", "remember", map[string]any{"fact": "User deploys on Fridays.", "shared": true})},
		{contentChunk("Noted.")},
		{toolChunk("call_rec", "recall", map[string]string{"query": "deploys"})},
		{contentChunk("Friday deploys.")},
	})
	st, err := store.Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	ctx := context.Background()
	bot, err := st.CreateBot(ctx, "Tester", "Be brief.", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := st.CreateThread(ctx, bot.ID, "Chat")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine := NewEngine(st, llm.New(scripted.URL, ""), bus.New(), config.Provider{Model: "test-model"}, t.TempDir(), config.ApprovalNever, log)
	run := func(prompt string) store.Run {
		t.Helper()
		if _, err := st.AddMessage(ctx, store.Message{ThreadID: thread.ID, Role: "user", Content: prompt}); err != nil {
			t.Fatalf("add message: %v", err)
		}
		next, err := st.CreateRun(ctx, bot.ID, thread.ID, "user", prompt)
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		if err := engine.Execute(ctx, next.ID); err != nil {
			t.Fatalf("execute: %v", err)
		}
		return next
	}
	first := run("remember the deploy day")
	stored, err := st.GetRun(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != store.RunDone {
		t.Fatalf("first run status %q", stored.Status)
	}
	memories, err := st.ListMemories(ctx, bot.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 1 || memories[0].Content != "User deploys on Fridays." {
		t.Fatalf("memories %+v", memories)
	}
	second := run("when do we deploy?")
	finished, err := st.GetRun(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != store.RunDone {
		t.Fatalf("second run status %q", finished.Status)
	}
	messages, err := st.ListMessages(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sawRecallResult bool
	for _, msg := range messages {
		if msg.Role == "tool" && msg.ToolName == "recall" && strings.Contains(msg.Content, "Fridays") {
			sawRecallResult = true
		}
	}
	if !sawRecallResult {
		t.Error("the second run never recalled the remembered fact")
	}
}

func newScriptedServer(t *testing.T, script [][]string) *httptest.Server {
	t.Helper()
	bank := &fakeProvider{script: script}
	server := httptest.NewServer(bank)
	t.Cleanup(server.Close)
	return server
}

func TestRememberedFactsReachTheSystemPrompt(t *testing.T) {
	h := newHarness(t, config.ApprovalNever, [][]string{{contentChunk("ok")}})
	ctx := context.Background()
	if _, err := h.store.AddMemory(ctx, "", "Shared fact."); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddMemory(ctx, h.bot.ID, "Bot fact."); err != nil {
		t.Fatal(err)
	}
	prompt := h.engine.systemPrompt(ctx, h.bot, store.Run{BotID: h.bot.ID, Trigger: "user"})
	if !strings.Contains(prompt, "Shared fact.") || !strings.Contains(prompt, "Bot fact.") {
		t.Errorf("system prompt is missing remembered facts:\n%s", prompt)
	}
}

func TestUnknownSkillExplainsItself(t *testing.T) {
	h := newHarness(t, config.ApprovalNever, [][]string{{contentChunk("ok")}})
	ctx := context.Background()
	if _, err := h.store.CreateSkill(ctx, "review", "Code review", "Check tests first."); err != nil {
		t.Fatal(err)
	}
	out, err := memory.Invoke(ctx, h.store, h.bot.ID, "/missing", "x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Unknown skill") || !strings.Contains(out, "/review") {
		t.Errorf("unexpected answer %q", out)
	}
}

func TestBuildMessagesCompactsOverBudget(t *testing.T) {
	h := newHarness(t, config.ApprovalNever, [][]string{{contentChunk("ok")}})
	ctx := context.Background()
	bot := h.bot
	var history []store.Message
	for i := 0; i < compactKeep+10; i++ {
		history = append(history, store.Message{Role: "user", Content: strings.Repeat("x", 5000)})
	}
	run := store.Run{BotID: bot.ID, ThreadID: h.thread.ID, Trigger: "user"}
	messages := h.engine.buildMessages(ctx, bot, run, history)
	if len(messages) < 3 {
		t.Fatalf("got %d messages, want system + summary + tail", len(messages))
	}
	if messages[0].Role != "system" {
		t.Errorf("first message role %q, want system", messages[0].Role)
	}
	if messages[1].Role != "user" || !strings.HasPrefix(messages[1].Content, store.SummaryMarker) {
		t.Errorf("second message is not the summary: %+v", messages[1])
	}
	userCount := 0
	for _, msg := range messages[2:] {
		if msg.Role == "user" {
			userCount++
		}
		if len(msg.Content) > 5000 {
			t.Errorf("kept message holds %d chars, budget dropped rows before reaching it", len(msg.Content))
		}
	}
	if userCount != compactKeep {
		t.Errorf("kept %d user messages, want %d", userCount, compactKeep)
	}
}

func TestBuildMessagesKeepsShortHistoryUntouched(t *testing.T) {
	h := newHarness(t, config.ApprovalNever, [][]string{{contentChunk("ok")}})
	ctx := context.Background()
	history := []store.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	run := store.Run{BotID: h.bot.ID, ThreadID: h.thread.ID, Trigger: "user"}
	messages := h.engine.buildMessages(ctx, h.bot, run, history)
	if len(messages) != 3 {
		t.Fatalf("got %d messages, want system + 2", len(messages))
	}
	if messages[1].Content != "hi" || messages[2].Content != "hello" {
		t.Errorf("short history was rewritten: %+v", messages[1:])
	}
}

func TestModelRetrySurvivesTransientFailures(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	bot, err := st.CreateBot(ctx, "Tester", "Be brief.", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := st.CreateThread(ctx, bot.ID, "Chat")
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `{"error":{"message":"blip"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: %s\n\n", contentChunk("recovered"))
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := llm.New(server.URL, "")
	engine := NewEngine(st, client, bus.New(), config.Provider{Model: "test-model"}, t.TempDir(), config.ApprovalNever, log)
	if _, err := st.AddMessage(ctx, store.Message{ThreadID: thread.ID, Role: "user", Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateRun(ctx, bot.ID, thread.ID, "user", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Execute(ctx, run.ID); err != nil {
		t.Fatalf("execute: %v", err)
	}
	stored, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != store.RunDone {
		t.Fatalf("status %q, want done (%s)", stored.Status, stored.Error)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 3 {
		t.Fatalf("provider called %d times, want 3 (2 retries)", calls)
	}
}
