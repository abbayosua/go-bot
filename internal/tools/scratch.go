package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gobot/internal/llm"
	"gobot/internal/store"
)

// Scratchpad tools keep working notes readable mid-run without polluting
// long-term memory: note appends, notes reads, clear_notes wipes.
type scratchKey struct{}

func scratchGet(ctx context.Context, st *store.Store, botID string) string {
	notes, err := st.GetSetting(ctx, "scratch:"+botID)
	if err != nil || notes == "" {
		return ""
	}
	return notes
}

type noteTool struct{ store *store.Store }

func (t *noteTool) Name() string { return "note" }

func (t *noteTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "note",
		Description: "Jot a working note for this bot (a lead, a partial result, a next step). Notes guide later runs and are NOT long-term memory; use remember for durable facts.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {"text": {"type": "string", "description": "One note line."}},
			"required": ["text"]
		}`),
	}}
}

func (t *noteTool) RunFor(ctx context.Context, botID string, args json.RawMessage) (string, error) {
	var input struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return "", errors.New("text is required")
	}
	existing := scratchGet(ctx, t.store, botID)
	combined := existing
	if combined != "" {
		combined += "\n"
	}
	combined += "- " + text
	if len(combined) > 8000 {
		combined = combined[len(combined)-8000:]
	}
	if err := t.store.SetSetting(ctx, "scratch:"+botID, combined); err != nil {
		return "", err
	}
	return "noted", nil
}

type notesTool struct{ store *store.Store }

func (t *notesTool) Name() string { return "notes" }

func (t *notesTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "notes",
		Description: "Read this bot's working notes.",
		Parameters:  json.RawMessage(`{"type": "object", "properties": {}}`),
	}}
}

func (t *notesTool) RunFor(ctx context.Context, botID string, _ json.RawMessage) (string, error) {
	if notes := scratchGet(ctx, t.store, botID); notes != "" {
		return notes, nil
	}
	return "no notes yet", nil
}

type clearNotesTool struct{ store *store.Store }

func (t *clearNotesTool) Name() string { return "clear_notes" }

func (t *clearNotesTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "clear_notes",
		Description: "Wipe this bot's working notes.",
		Parameters:  json.RawMessage(`{"type": "object", "properties": {}}`),
	}}
}

func (t *clearNotesTool) RunFor(ctx context.Context, botID string, _ json.RawMessage) (string, error) {
	if err := t.store.SetSetting(ctx, "scratch:"+botID, ""); err != nil {
		return "", err
	}
	return "notes cleared", nil
}

// ScratchBlock returns the prompt block for one bot's notes.
func ScratchBlock(ctx context.Context, st *store.Store, botID string) string {
	return scratchGet(ctx, st, botID)
}

var _ = scratchKey{}
