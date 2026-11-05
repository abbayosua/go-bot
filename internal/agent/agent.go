package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gobot/internal/bus"
	"gobot/internal/config"
	"gobot/internal/llm"
	"gobot/internal/memory"
	"gobot/internal/store"
	"gobot/internal/tools"
)

const (
	MaxSteps        = 24
	historyLimit    = 80
	loopGuardLimit  = 6
	leaseDuration   = 5 * time.Minute
	approvalSetting = "approval_mode"
)

var readOnlyTools = map[string]bool{
	"recall":     true,
	"read_file":  true,
	"list_files": true,
	"web_fetch":  true,
}

type Engine struct {
	Store      *store.Store
	Client     *llm.Client
	Bus        *bus.Bus
	Provider   config.Provider
	Workspaces string
	Default    string
	Log        *slog.Logger

	mu     sync.Mutex
	active map[string]bool
}

func NewEngine(st *store.Store, client *llm.Client, b *bus.Bus, provider config.Provider, workspaces, defaultMode string, log *slog.Logger) *Engine {
	return &Engine{
		Store: st, Client: client, Bus: b, Provider: provider,
		Workspaces: workspaces, Default: defaultMode, Log: log,
		active: map[string]bool{},
	}
}

func (e *Engine) Start(runID string) {
	go func() {
		if err := e.Execute(context.Background(), runID); err != nil {
			e.Log.Error("run failed", "run", runID, "error", err)
		}
	}()
}

func (e *Engine) begin(runID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.active[runID] {
		return false
	}
	e.active[runID] = true
	return true
}

func (e *Engine) end(runID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.active, runID)
}

// Execute runs one turn to completion, pausing at an approval or a cancellation.
func (e *Engine) Execute(ctx context.Context, runID string) error {
	if !e.begin(runID) {
		return nil
	}
	defer e.end(runID)

	run, err := e.Store.ClaimRun(ctx, runID, leaseDuration)
	if err != nil {
		return err
	}
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = e.Store.ReleaseRun(release, runID)
	}()

	if run.Status == store.RunDone || run.Status == store.RunCancelled {
		return nil
	}
	bot, err := e.Store.GetBot(ctx, run.BotID)
	if err != nil {
		return e.fail(ctx, run, err, run.Steps)
	}
	thread, err := e.Store.GetThread(ctx, run.ThreadID)
	if err != nil {
		return e.fail(ctx, run, err, run.Steps)
	}
	workspace := filepath.Join(e.Workspaces, bot.ID)
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		return e.fail(ctx, run, err, run.Steps)
	}
	registry := tools.WrapMemory(tools.NewRegistry(workspace), e.Store)
	mode := e.approvalMode(ctx)
	steps := run.Steps
	guard := newLoopGuard()

	if err := e.setStatus(ctx, run, store.RunRunning, "", steps); err != nil {
		return err
	}

	for steps < MaxSteps {
		if err := ctx.Err(); err != nil {
			return e.cancel(ctx, run, steps)
		}
		current, err := e.Store.GetRun(ctx, run.ID)
		if err != nil {
			return err
		}
		if current.Status == store.RunCancelled {
			return nil
		}

		history, err := e.Store.ListMessages(ctx, thread.ID)
		if err != nil {
			return e.fail(ctx, run, err, steps)
		}

		pending := missingToolCalls(history)
		if len(pending) > 0 {
			outcome, err := e.runTools(ctx, run, registry, mode, pending, guard)
			if err != nil {
				return e.fail(ctx, run, err, steps)
			}
			switch outcome {
			case toolsPaused:
				return nil
			case toolsStopped:
				return e.finish(ctx, run, steps)
			}
			continue
		}

		result, err := e.callModel(ctx, run, bot, thread, registry)
		if err != nil {
			return e.fail(ctx, run, err, steps)
		}
		steps++
		if err := e.setStatus(ctx, run, store.RunRunning, "", steps); err != nil {
			return err
		}
		if stopped := guard.observeModelResult(result); stopped != "" {
			if _, err := e.Store.AddMessage(ctx, store.Message{
				ThreadID: thread.ID, Role: "assistant", Content: stopped,
			}); err != nil {
				return e.fail(ctx, run, err, steps)
			}
			return e.finish(ctx, run, steps)
		}
		if len(result.ToolCalls) == 0 {
			return e.finish(ctx, run, steps)
		}
	}

	notice := fmt.Sprintf("I stopped after %d steps without finishing. Send another message to continue.", MaxSteps)
	msg, err := e.Store.AddMessage(ctx, store.Message{ThreadID: thread.ID, Role: "assistant", Content: notice})
	if err != nil {
		return e.fail(ctx, run, err, steps)
	}
	e.publish(thread.ID, bus.Event{Type: "message", Data: msg})
	return e.finish(ctx, run, steps)
}

func (e *Engine) callModel(ctx context.Context, run store.Run, bot store.Bot, thread store.Thread, registry *tools.MemoryRegistry) (llm.Result, error) {
	history, err := e.Store.ListMessages(ctx, thread.ID)
	if err != nil {
		return llm.Result{}, err
	}
	messages := e.buildMessages(ctx, bot, run, history)
	model := bot.Model
	if model == "" {
		model = e.Provider.Model
	}
	result, err := e.Client.Stream(ctx, model, messages, registry.Specs(), func(text string) {
		e.publish(thread.ID, bus.Event{Type: "delta", Data: map[string]string{"text": text}})
	})
	if err != nil {
		return llm.Result{}, err
	}
	stored, err := e.Store.AddMessage(ctx, store.Message{
		ThreadID:  thread.ID,
		Role:      "assistant",
		Content:   result.Content,
		ToolCalls: marshalCalls(result.ToolCalls),
	})
	if err != nil {
		return llm.Result{}, err
	}
	e.publish(thread.ID, bus.Event{Type: "message", Data: stored})
	return result, nil
}

func (e *Engine) buildMessages(ctx context.Context, bot store.Bot, run store.Run, history []store.Message) []llm.Message {
	messages := []llm.Message{{Role: "system", Content: e.systemPrompt(ctx, bot, run)}}
	start := 0
	if len(history) > historyLimit {
		start = len(history) - historyLimit
	}
	for _, msg := range history[start:] {
		switch msg.Role {
		case "user":
			messages = append(messages, llm.Message{Role: "user", Content: msg.Content})
		case "assistant":
			messages = append(messages, llm.Message{
				Role:      "assistant",
				Content:   msg.Content,
				ToolCalls: parseCalls(msg.ToolCalls),
			})
		case "tool":
			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
			})
		}
	}
	if run.Prompt != "" && !lastUserMatches(history, run.Prompt) {
		messages = append(messages, llm.Message{Role: "user", Content: run.Prompt})
	}
	return messages
}

func lastUserMatches(history []store.Message, prompt string) bool {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			return history[i].Content == prompt
		}
	}
	return false
}

func (e *Engine) systemPrompt(ctx context.Context, bot store.Bot, run store.Run) string {
	var b strings.Builder
	b.WriteString("You are " + bot.Name + ", an autonomous agent running on the user's machine.\n")
	if strings.TrimSpace(bot.Instructions) != "" {
		b.WriteString("\nStanding instructions:\n" + bot.Instructions + "\n")
	}
	workspace := filepath.Join(e.Workspaces, bot.ID)
	if block := memory.Block(ctx, e.Store, bot.ID); block != "" {
		b.WriteString("\nWhat you remember:\n" + block + "\n")
	}
	if names := tools.SkillLine(ctx, e.Store); names != "" {
		b.WriteString("\nSkills available over chat: " + names +
			" — a user typing one expects its contents answered from the shared playbook.\n")
	}
	b.WriteString("\nWorkspace: " + workspace + "\n")
	b.WriteString("Current time: " + time.Now().Format(time.RFC1123) + "\n")
	b.WriteString("Run started by: " + run.Trigger + "\n")
	b.WriteString(`
Work the task to completion using the tools. Rules:
- Read before you write; verify with a command or a file read instead of assuming.
- Use remember for durable facts and recall before asking the user something answered before.
- Never claim a result you did not observe. If a tool fails, say what failed and try another approach.
- Keep answers short. Return only what the user needs.
- Files you create live in the workspace. Report their paths.`)
	return b.String()
}

func marshalCalls(calls []llm.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	raw, err := json.Marshal(calls)
	if err != nil {
		return ""
	}
	return string(raw)
}

func parseCalls(raw string) []llm.ToolCall {
	if raw == "" {
		return nil
	}
	var calls []llm.ToolCall
	if err := json.Unmarshal([]byte(raw), &calls); err != nil {
		return nil
	}
	return calls
}

// missingToolCalls returns calls from the last assistant turn that have no
// result yet. The message history is the checkpoint: a paused run resumes here.
func missingToolCalls(history []store.Message) []llm.ToolCall {
	lastAssistant := -1
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "assistant" && history[i].ToolCalls != "" {
			lastAssistant = i
			break
		}
	}
	if lastAssistant < 0 {
		return nil
	}
	calls := parseCalls(history[lastAssistant].ToolCalls)
	if len(calls) == 0 {
		return nil
	}
	answered := map[string]bool{}
	for _, msg := range history[lastAssistant+1:] {
		if msg.Role == "tool" && msg.ToolCallID != "" {
			answered[msg.ToolCallID] = true
		}
	}
	var pending []llm.ToolCall
	for _, call := range calls {
		if !answered[call.ID] {
			pending = append(pending, call)
		}
	}
	return pending
}

type toolOutcome int

const (
	toolsContinue toolOutcome = iota
	toolsPaused
	toolsStopped
)

func (e *Engine) runTools(ctx context.Context, run store.Run, registry *tools.MemoryRegistry, mode string, calls []llm.ToolCall, guard *loopGuard) (toolOutcome, error) {
	for _, call := range calls {
		if err := ctx.Err(); err != nil {
			return toolsContinue, err
		}
		if !registry.Has(call.Function.Name) {
			if err := e.recordToolError(ctx, run, call, fmt.Sprintf("unknown tool %q", call.Function.Name)); err != nil {
				return toolsContinue, err
			}
			continue
		}
		decision, err := e.decide(ctx, run, mode, call)
		if err != nil {
			return toolsContinue, err
		}
		switch decision {
		case store.ApprovalDeny:
			if err := e.recordToolError(ctx, run, call, "the user denied this tool call"); err != nil {
				return toolsContinue, err
			}
			continue
		case store.ApprovalAsk:
			approval, err := e.Store.CreateApproval(ctx, run.ID, run.ThreadID, call.ID, call.Function.Name, call.Function.Arguments)
			if err != nil {
				return toolsContinue, err
			}
			stored, err := e.Store.GetApproval(ctx, approval.ID)
			if err != nil {
				return toolsContinue, err
			}
			if err := e.setStatus(ctx, run, store.RunWaitingApproval, "", run.Steps); err != nil {
				return toolsContinue, err
			}
			e.publish(run.ThreadID, bus.Event{Type: "approval", Data: stored})
			return toolsPaused, nil
		}

		guard.observeToolCall(call)
		if stop := guard.stopText(); stop != "" {
			msg, err := e.Store.AddMessage(ctx, store.Message{ThreadID: run.ThreadID, Role: "assistant", Content: stop})
			if err != nil {
				return toolsContinue, err
			}
			e.publish(run.ThreadID, bus.Event{Type: "message", Data: msg})
			return toolsStopped, nil
		}

		output, runErr := registry.Run(ctx, run.BotID, call.Function.Name, json.RawMessage(call.Function.Arguments))
		if runErr != nil {
			output = "error: " + runErr.Error()
		}
		msg, err := e.Store.AddMessage(ctx, store.Message{
			ThreadID:   run.ThreadID,
			Role:       "tool",
			Content:    output,
			ToolCallID: call.ID,
			ToolName:   call.Function.Name,
		})
		if err != nil {
			return toolsContinue, err
		}
		e.publish(run.ThreadID, bus.Event{Type: "tool", Data: msg})
	}
	return toolsContinue, nil
}

func (e *Engine) recordToolError(ctx context.Context, run store.Run, call llm.ToolCall, reason string) error {
	msg, err := e.Store.AddMessage(ctx, store.Message{
		ThreadID:   run.ThreadID,
		Role:       "tool",
		Content:    "error: " + reason,
		ToolCallID: call.ID,
		ToolName:   call.Function.Name,
	})
	if err != nil {
		return err
	}
	e.publish(run.ThreadID, bus.Event{Type: "tool", Data: msg})
	return nil
}

// decide applies the approval mode. Read-only tools never wait; a recorded
// decision always wins so a resumed run does not ask twice.
func (e *Engine) decide(ctx context.Context, run store.Run, mode string, call llm.ToolCall) (string, error) {
	existing, err := e.Store.ApprovalForCall(ctx, run.ID, call.ID)
	if err == nil {
		if existing.Status == store.ApprovalPending {
			return store.ApprovalAsk, nil
		}
		return existing.Status, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	switch mode {
	case config.ApprovalNever:
		return store.ApprovalAllow, nil
	case config.ApprovalAuto:
		if readOnlyTools[call.Function.Name] {
			return store.ApprovalAllow, nil
		}
		if e.Provider.JudgeModel == "" {
			return store.ApprovalAsk, nil
		}
		verdict, judgeErr := e.judge(ctx, run, call)
		if judgeErr != nil {
			e.Log.Warn("approval judge failed; asking instead", "run", run.ID, "error", judgeErr)
			return store.ApprovalAsk, nil
		}
		return verdict, nil
	default:
		if readOnlyTools[call.Function.Name] {
			return store.ApprovalAllow, nil
		}
		return store.ApprovalAsk, nil
	}
}

func (e *Engine) judge(ctx context.Context, run store.Run, call llm.ToolCall) (string, error) {
	prompt := fmt.Sprintf(`The user asked for: %s

The agent wants to call:
tool: %s
arguments: %s

Decide whether this action follows from the user's request. Answer with one word: ALLOW or DENY.`,
		run.Prompt, call.Function.Name, call.Function.Arguments)
	answer, err := e.Client.Complete(ctx, e.Provider.JudgeModel, []llm.Message{
		{Role: "system", Content: "You approve or deny one tool call. Be strict about destructive or off-task actions."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return "", err
	}
	if strings.Contains(strings.ToUpper(answer), "ALLOW") {
		return store.ApprovalAllow, nil
	}
	return store.ApprovalAsk, nil
}

func (e *Engine) approvalMode(ctx context.Context) string {
	value, err := e.Store.GetSetting(ctx, approvalSetting)
	if err == nil && value != "" {
		return value
	}
	if e.Default != "" {
		return e.Default
	}
	return config.ApprovalAsk
}

func (e *Engine) ApprovalMode(ctx context.Context) string { return e.approvalMode(ctx) }

func (e *Engine) setStatus(ctx context.Context, run store.Run, status, errText string, steps int) error {
	if err := e.Store.UpdateRun(ctx, run.ID, status, errText, steps); err != nil {
		return err
	}
	updated, err := e.Store.GetRun(ctx, run.ID)
	if err != nil {
		return err
	}
	e.publish(run.ThreadID, bus.Event{Type: "run", Data: updated})
	return nil
}

func (e *Engine) finish(ctx context.Context, run store.Run, steps int) error {
	return e.setStatus(ctx, run, store.RunDone, "", steps)
}

func (e *Engine) cancel(ctx context.Context, run store.Run, steps int) error {
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return e.setStatus(stopCtx, run, store.RunCancelled, "", steps)
}

func (e *Engine) fail(ctx context.Context, run store.Run, cause error, steps int) error {
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.setStatus(stopCtx, run, store.RunFailed, cause.Error(), steps); err != nil {
		e.Log.Error("record run failure", "run", run.ID, "error", err)
	}
	msg, err := e.Store.AddMessage(stopCtx, store.Message{
		ThreadID: run.ThreadID,
		Role:     "assistant",
		Content:  "I could not finish: " + cause.Error(),
	})
	if err == nil {
		e.publish(run.ThreadID, bus.Event{Type: "message", Data: msg})
	}
	return cause
}

func (e *Engine) publish(threadID string, event bus.Event) {
	if e.Bus != nil {
		e.Bus.Publish(threadID, event)
	}
}

// loopGuard stops a model that repeats one call instead of making progress.
type loopGuard struct {
	key   string
	count int
}

func newLoopGuard() *loopGuard { return &loopGuard{} }

func (g *loopGuard) observeToolCall(call llm.ToolCall) {
	key := call.Function.Name + "\x00" + call.Function.Arguments
	if key == g.key {
		g.count++
		return
	}
	g.key = key
	g.count = 1
}

func (g *loopGuard) stopText() string {
	if g.count < loopGuardLimit {
		return ""
	}
	name := strings.SplitN(g.key, "\x00", 2)[0]
	return fmt.Sprintf("I got stuck calling %s with the same input %d times without progress, so I stopped. Tell me a different approach and I will continue.", name, g.count)
}

func (g *loopGuard) observeModelResult(result llm.Result) string {
	if len(result.ToolCalls) > 0 {
		return ""
	}
	g.key = ""
	g.count = 0
	return ""
}
