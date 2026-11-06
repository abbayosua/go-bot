package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gobot/internal/core"
	"gobot/internal/llm"
	"gobot/internal/store"
)

// askBot lets one bot read through another bot: the target's memories,
// knowledge pages and recent thread context are searched for the question.
// The answer cites its source bot, and nothing is executed on the target.
type askBotTool struct {
	store    *store.Store
	memories core.MemoryProvider
}

func (t *askBotTool) Name() string { return "ask_bot" }

func (t *askBotTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "ask_bot",
		Description: "Ask another bot what it knows. Searches its memories, knowledge pages and recent messages, and cites the source. Use this instead of guessing what a peer bot holds.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"bot": {"type": "string", "description": "Peer bot name or id."},
				"question": {"type": "string", "description": "What to ask."}
			},
			"required": ["bot", "question"]
		}`),
	}}
}

func (t *askBotTool) RunFor(ctx context.Context, botID string, args json.RawMessage) (string, error) {
	var input struct {
		Bot      string `json:"bot"`
		Question string `json:"question"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Bot) == "" || strings.TrimSpace(input.Question) == "" {
		return "", errors.New("bot and question are required")
	}
	target, err := t.store.FindBot(ctx, strings.TrimSpace(input.Bot))
	if err != nil {
		return "", err
	}
	if target.ID == botID {
		return "", errors.New("ask another bot, not yourself")
	}
	memories, err := t.memories.Search(ctx, target.ID, input.Question, 5)
	if err != nil {
		return "", err
	}
	pages, err := t.store.SearchKnowledge(ctx, target.ID, input.Question, 3)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, m := range memories {
		lines = append(lines, "- memory: "+m.Content)
	}
	for _, p := range pages {
		body := p.Body
		if len(body) > 300 {
			body = body[:300] + "…"
		}
		lines = append(lines, "- page "+p.Title+": "+body)
	}
	if len(lines) == 0 {
		return target.Name + " holds nothing on that.", nil
	}
	return "From " + target.Name + ":\n" + strings.Join(lines, "\n"), nil
}
