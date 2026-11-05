package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gobot/internal/llm"
	"gobot/internal/memory"
	"gobot/internal/store"
)

type rememberTool struct{ store *store.Store }

func (t *rememberTool) Name() string { return "remember" }

func (t *rememberTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "remember",
		Description: "Save a durable fact: a preference, a decision, or something you always need true. Set shared to also make it visible to every bot.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"fact": {"type": "string", "description": "One self-contained fact, written so it still makes sense later."},
				"shared": {"type": "boolean", "description": "True to share this fact with every bot. Defaults to false."}
			},
			"required": ["fact"]
		}`),
	}}
}

func (t *rememberTool) RunFor(ctx context.Context, botID string, args json.RawMessage) (string, error) {
	var input struct {
		Fact   string `json:"fact"`
		Shared bool   `json:"shared"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Fact) == "" {
		return "", errors.New("fact is required")
	}
	scope := botID
	if input.Shared {
		scope = ""
	}
	if _, err := t.store.AddMemory(ctx, scope, strings.TrimSpace(input.Fact)); err != nil {
		return "", err
	}
	if input.Shared {
		return "remembered for every bot", nil
	}
	return "remembered", nil
}

type recallTool struct{ store *store.Store }

func (t *recallTool) Name() string { return "recall" }

func (t *recallTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "recall",
		Description: "Search what was remembered. Use this before asking the user something answered before.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "Words to look for, e.g. \"smtp port\" or \"deploy checklist\"."}
			},
			"required": ["query"]
		}`),
	}}
}

func (t *recallTool) RunFor(ctx context.Context, botID string, args json.RawMessage) (string, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	memories, err := t.store.SearchMemories(ctx, botID, input.Query, 10)
	if err != nil {
		return "", err
	}
	if len(memories) == 0 {
		return "nothing remembered matches", nil
	}
	lines := make([]string, 0, len(memories))
	for _, m := range memories {
		lines = append(lines, "- "+m.Content)
	}
	return strings.Join(lines, "\n"), nil
}

// Registry adds the memory-scoped tools to a workspace registry. Calls carry
// the bot id so scoped facts land on the right owner.
type MemoryRegistry struct {
	inner botRegistry
	store *store.Store
}

type botRegistry interface {
	Has(name string) bool
	Run(ctx context.Context, name string, args json.RawMessage) (string, error)
	Specs() []llm.Tool
}

func WrapMemory(inner botRegistry, st *store.Store) *MemoryRegistry {
	return &MemoryRegistry{inner: inner, store: st}
}

func (r *MemoryRegistry) Has(name string) bool {
	return name == "remember" || name == "recall" || r.inner.Has(name)
}

func (r *MemoryRegistry) Specs() []llm.Tool {
	specs := r.inner.Specs()
	specs = append(specs,
		(&rememberTool{store: r.store}).Spec(),
		(&recallTool{store: r.store}).Spec())
	return specs
}

func (r *MemoryRegistry) Run(ctx context.Context, botID, name string, args json.RawMessage) (string, error) {
	switch name {
	case "remember":
		return (&rememberTool{store: r.store}).RunFor(ctx, botID, args)
	case "recall":
		return (&recallTool{store: r.store}).RunFor(ctx, botID, args)
	default:
		return r.inner.Run(ctx, name, args)
	}
}

// SkillLine describes the callable skills for the system prompt.
func SkillLine(ctx context.Context, st *store.Store) string {
	skills, err := st.ListSkills(ctx)
	if err != nil {
		return ""
	}
	return memory.SkillNames(skills)
}
