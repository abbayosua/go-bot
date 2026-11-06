package tools

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"gobot/internal/core"
	"gobot/internal/llm"
	"gobot/internal/memory"
	"gobot/internal/store"
)

type rememberTool struct{ memories core.MemoryProvider }

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
	if _, err := t.memories.Add(ctx, scope, strings.TrimSpace(input.Fact)); err != nil {
		return "", err
	}
	if input.Shared {
		return "remembered for every bot", nil
	}
	return "remembered", nil
}

type recallTool struct{ memories core.MemoryProvider }

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
	memories, err := t.memories.Search(ctx, botID, input.Query, 10)
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
	inner    botRegistry
	store    *store.Store
	memories core.MemoryProvider
	delegate DelegateFunc
	extra    map[string]Tool
	runID    string
}

type botRegistry interface {
	Has(name string) bool
	Run(ctx context.Context, name string, args json.RawMessage) (string, error)
	Specs() []llm.Tool
}

func WrapMemory(inner botRegistry, st *store.Store) *MemoryRegistry {
	provider := core.MemoryProvider(st.Memories())
	return &MemoryRegistry{inner: inner, store: st, memories: provider}
}

// WithRunID tags the parent run for tools that track per-run state.
func (r *MemoryRegistry) WithRunID(runID string) *MemoryRegistry {
	r.runID = runID
	return r
}

// WithMemoryProvider swaps the memory backend: an external adapter plugs in
// here, and the agent loop never changes.
func (r *MemoryRegistry) WithMemoryProvider(provider core.MemoryProvider) *MemoryRegistry {
	r.memories = provider
	return r
}

func (r *MemoryRegistry) Has(name string) bool {
	switch name {
	case "remember", "recall", "note", "notes", "clear_notes", "ask_bot", "delegate", "use_secret", "delegate_external",
		"fleet_create", "fleet_rename", "fleet_instruct", "fleet_schedule", "fleet_list":
		return true
	}
	if _, ok := r.extra[name]; ok {
		return true
	}
	return r.inner.Has(name)
}

func (r *MemoryRegistry) Specs() []llm.Tool {
	specs := r.inner.Specs()
	specs = append(specs,
		(&rememberTool{memories: r.memories}).Spec(),
		(&recallTool{memories: r.memories}).Spec(),
		(&noteTool{store: r.store}).Spec(),
		(&notesTool{store: r.store}).Spec(),
		(&clearNotesTool{store: r.store}).Spec(),
		(&askBotTool{store: r.store, memories: r.memories}).Spec(),
		(&delegateTool{}).Spec(),
		(&externalTool{}).Spec(),
		(&useSecretTool{}).Spec(),
		(&fleetCreateTool{}).Spec(),
		(&fleetRenameTool{}).Spec(),
		(&fleetInstructTool{}).Spec(),
		(&fleetScheduleTool{}).Spec(),
		(&fleetListTool{}).Spec())
	extraNames := make([]string, 0, len(r.extra))
	for name := range r.extra {
		extraNames = append(extraNames, name)
	}
	sort.Strings(extraNames)
	for _, name := range extraNames {
		specs = append(specs, r.extra[name].Spec())
	}
	return specs
}

func (r *MemoryRegistry) Run(ctx context.Context, botID, name string, args json.RawMessage) (string, error) {
	if extra, ok := r.extra[name]; ok {
		return extra.Run(ctx, args)
	}
	switch name {
	case "remember":
		return (&rememberTool{memories: r.memories}).RunFor(ctx, botID, args)
	case "recall":
		return (&recallTool{memories: r.memories}).RunFor(ctx, botID, args)
	case "note":
		return (&noteTool{store: r.store}).RunFor(ctx, botID, args)
	case "notes":
		return (&notesTool{store: r.store}).RunFor(ctx, botID, args)
	case "clear_notes":
		return (&clearNotesTool{store: r.store}).RunFor(ctx, botID, args)
	case "ask_bot":
		return (&askBotTool{store: r.store, memories: r.memories}).RunFor(ctx, botID, args)
	case "delegate":
		return (&delegateTool{run: r.delegate}).RunFor(ctx, botID, args)
	case "delegate_external":
		return (&externalTool{store: r.store}).RunFor(WithExternalRun(ctx, r.runID), botID, args)
	case "use_secret":
		return (&useSecretTool{registry: r}).RunFor(ctx, botID, args)
	case "fleet_create", "fleet_rename", "fleet_instruct", "fleet_schedule", "fleet_list":
		gate := &fleetGate{store: r.store}
		switch name {
		case "fleet_create":
			return (&fleetCreateTool{store: r.store, gate: gate}).RunFor(ctx, botID, args)
		case "fleet_rename":
			return (&fleetRenameTool{store: r.store, gate: gate}).RunFor(ctx, botID, args)
		case "fleet_instruct":
			return (&fleetInstructTool{store: r.store, gate: gate}).RunFor(ctx, botID, args)
		case "fleet_schedule":
			return (&fleetScheduleTool{store: r.store, gate: gate}).RunFor(ctx, botID, args)
		default:
			return (&fleetListTool{store: r.store, gate: gate, memories: r.memories}).RunFor(ctx, botID, args)
		}
	default:
		return r.inner.Run(ctx, name, args)
	}
}

// WithDelegate wires the hand-off executor: the agent loop injects a bound
// delegate that knows the parent run, keeping depth/count bounds in agent.
func (r *MemoryRegistry) WithDelegate(fn DelegateFunc) *MemoryRegistry {
	r.delegate = fn
	return r
}

// WithExtra mounts live tools (MCP bridge results): they appear in Specs,
// answer Has, and run before the inner registry.
func (r *MemoryRegistry) WithExtra(extra []Tool) *MemoryRegistry {
	if r.extra == nil {
		r.extra = map[string]Tool{}
	}
	for _, tool := range extra {
		r.extra[tool.Name()] = tool
	}
	return r
}

// SkillLine describes the callable skills for the system prompt.
func SkillLine(ctx context.Context, st *store.Store) string {
	skills, err := st.ListSkills(ctx)
	if err != nil {
		return ""
	}
	return memory.SkillNames(skills)
}
