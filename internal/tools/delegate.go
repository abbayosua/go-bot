package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gobot/internal/llm"
)

// DelegateFunc runs a hand-off: the agent loop injects it, so the tool stays
// a thin spec while execution (and its depth/count bounds) lives in agent.
type DelegateFunc func(ctx context.Context, target, task string, fresh bool) (string, error)

type delegateTool struct {
	run DelegateFunc
}

func (t *delegateTool) Name() string { return "delegate" }

func (t *delegateTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "delegate",
		Description: "Hand a task to a peer bot or a short-lived subagent and read the result back. Set fresh to spawn a one-off subagent named by bot. At most 3 delegations per run, 3 deep.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"bot": {"type": "string", "description": "Peer bot name/id, or the new subagent name when fresh is true."},
				"task": {"type": "string", "description": "The task to hand off."},
				"fresh": {"type": "boolean", "description": "True to spawn a short-lived subagent instead of using a peer bot."}
			},
			"required": ["bot", "task"]
		}`),
	}}
}

func (t *delegateTool) RunFor(ctx context.Context, botID string, args json.RawMessage) (string, error) {
	if t.run == nil {
		return "", errors.New("delegation is not wired for this run")
	}
	var input struct {
		Bot   string `json:"bot"`
		Task  string `json:"task"`
		Fresh bool   `json:"fresh"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Bot) == "" || strings.TrimSpace(input.Task) == "" {
		return "", errors.New("bot and task are required")
	}
	return t.run(ctx, strings.TrimSpace(input.Bot), strings.TrimSpace(input.Task), input.Fresh)
}
