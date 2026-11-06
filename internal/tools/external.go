package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gobot/internal/llm"
	"gobot/internal/store"
)

// externalTool hands a task to an outside agent service and reads the result
// back like a subagent's reply. The service receives {task, callback}; it
// POSTs {token, result} to the callback. The run polls briefly, then reports.
type externalTool struct {
	store *store.Store
}

func (t *externalTool) Name() string { return "delegate_external" }

func (t *externalTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "delegate_external",
		Description: "Hand a task to a registered outside agent service and read the result back. Use for work this install cannot do itself.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"agent": {"type": "string", "description": "Outside service name or id."},
				"task": {"type": "string", "description": "The task to hand off."}
			},
			"required": ["agent", "task"]
		}`),
	}}
}

func (t *externalTool) RunFor(ctx context.Context, botID string, args json.RawMessage) (string, error) {
	var input struct {
		Agent string `json:"agent"`
		Task  string `json:"task"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Task) == "" {
		return "", errors.New("task is required")
	}
	agent, err := t.store.FindExternalAgent(ctx, strings.TrimSpace(input.Agent))
	if err != nil {
		return "", err
	}
	// The parent run id rides alongside: override via context when present.
	runID, _ := ctx.Value(externalRunKey{}).(string)
	task, err := t.store.CreateExternalTask(ctx, agent.ID, runID, strings.TrimSpace(input.Task))
	if err != nil {
		return "", err
	}
	payload, _ := json.Marshal(map[string]any{
		"task": task.Task, "token": task.Token,
		"callback": "/api/xagents/in/" + task.Token,
	})
	dispatch, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(dispatch, http.MethodPost, agent.URL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("outside service unreachable: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("outside service status %d", resp.StatusCode)
	}
	// Wait for the callback like a subagent's reply: bounded, quiet.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		stored, err := t.store.ExternalTaskResult(ctx, runID, task.ID)
		if err == nil && stored.Status == "done" {
			agent, _ := t.store.FindExternalAgent(ctx, stored.AgentID)
			name := agent.Name
			if name == "" {
				name = "outside service"
			}
			if strings.TrimSpace(stored.Result) == "" {
				return "From " + name + ": (no result)", nil
			}
			return "From " + name + ": " + stored.Result, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return "", errors.New("outside service did not answer in time (the callback may still land later)")
}

type externalRunKey struct{}

// WithExternalRun tags the parent run id for task tracking.
func WithExternalRun(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, externalRunKey{}, runID)
}
