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

// Fleet tools let one bot manage other bots: create, rename, instruct and
// schedule. Every tool first checks the per-bot fleet switch
// (setting "fleet:<botID>" == "on"); the switch defaults to off, so a bot
// only wields fleet powers when its owner enables them.
type fleetGate struct {
	store *store.Store
}

func (g *fleetGate) allowed(ctx context.Context, botID string) bool {
	if g == nil || g.store == nil {
		return false
	}
	value, err := g.store.GetSetting(ctx, "fleet:"+botID)
	return err == nil && value == "on"
}

func (g *fleetGate) deny() error {
	return errors.New("fleet tools are off for this bot")
}

func fleetSpec(name, description, params string) llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name: name, Description: description, Parameters: json.RawMessage(params),
	}}
}

type fleetCreateTool struct {
	store *store.Store
	gate  *fleetGate
}

func (t *fleetCreateTool) Name() string { return "fleet_create" }

func (t *fleetCreateTool) Spec() llm.Tool {
	return fleetSpec(t.Name(),
		"Create a new bot (a teammate). Needs the fleet switch on.",
		`{"type":"object","properties":{"name":{"type":"string"},"instructions":{"type":"string"}},"required":["name"]}`)
}

func (t *fleetCreateTool) RunFor(ctx context.Context, botID string, args json.RawMessage) (string, error) {
	if !t.gate.allowed(ctx, botID) {
		return "", t.gate.deny()
	}
	var input struct {
		Name         string `json:"name"`
		Instructions string `json:"instructions"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Name) == "" {
		return "", errors.New("name is required")
	}
	me, err := t.store.GetBot(ctx, botID)
	if err != nil {
		return "", err
	}
	bot, err := t.store.CreateBotIn(ctx, me.SpaceID, strings.TrimSpace(input.Name), input.Instructions, "")
	if err != nil {
		return "", err
	}
	if _, err := t.store.CreateThread(ctx, bot.ID, "Chat"); err != nil {
		return "", err
	}
	return "created bot " + bot.Name + " (" + bot.ID + ")", nil
}

type fleetRenameTool struct {
	store *store.Store
	gate  *fleetGate
}

func (t *fleetRenameTool) Name() string { return "fleet_rename" }

func (t *fleetRenameTool) Spec() llm.Tool {
	return fleetSpec(t.Name(),
		"Rename a bot by name or id. Needs the fleet switch on.",
		`{"type":"object","properties":{"bot":{"type":"string"},"name":{"type":"string"}},"required":["bot","name"]}`)
}

func (t *fleetRenameTool) RunFor(ctx context.Context, botID string, args json.RawMessage) (string, error) {
	if !t.gate.allowed(ctx, botID) {
		return "", t.gate.deny()
	}
	var input struct {
		Bot  string `json:"bot"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Name) == "" {
		return "", errors.New("name is required")
	}
	target, err := t.store.FindBot(ctx, strings.TrimSpace(input.Bot))
	if err != nil {
		return "", err
	}
	if err := t.store.UpdateBot(ctx, target.ID, strings.TrimSpace(input.Name), target.Instructions, target.Model); err != nil {
		return "", err
	}
	return "renamed to " + strings.TrimSpace(input.Name), nil
}

type fleetInstructTool struct {
	store *store.Store
	gate  *fleetGate
}

func (t *fleetInstructTool) Name() string { return "fleet_instruct" }

func (t *fleetInstructTool) Spec() llm.Tool {
	return fleetSpec(t.Name(),
		"Rewrite another bot's standing instructions. Needs the fleet switch on.",
		`{"type":"object","properties":{"bot":{"type":"string"},"instructions":{"type":"string"}},"required":["bot","instructions"]}`)
}

func (t *fleetInstructTool) RunFor(ctx context.Context, botID string, args json.RawMessage) (string, error) {
	if !t.gate.allowed(ctx, botID) {
		return "", t.gate.deny()
	}
	var input struct {
		Bot          string `json:"bot"`
		Instructions string `json:"instructions"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	target, err := t.store.FindBot(ctx, strings.TrimSpace(input.Bot))
	if err != nil {
		return "", err
	}
	if err := t.store.UpdateBot(ctx, target.ID, target.Name, input.Instructions, target.Model); err != nil {
		return "", err
	}
	return "instructions updated for " + target.Name, nil
}

type fleetScheduleTool struct {
	store *store.Store
	gate  *fleetGate
}

func (t *fleetScheduleTool) Name() string { return "fleet_schedule" }

func (t *fleetScheduleTool) Spec() llm.Tool {
	return fleetSpec(t.Name(),
		"Schedule a routine on another bot from a cron expression. Needs the fleet switch on.",
		`{"type":"object","properties":{"bot":{"type":"string"},"prompt":{"type":"string"},"cron":{"type":"string"},"name":{"type":"string"}},"required":["bot","prompt","cron"]}`)
}

func (t *fleetScheduleTool) RunFor(ctx context.Context, botID string, args json.RawMessage) (string, error) {
	if !t.gate.allowed(ctx, botID) {
		return "", t.gate.deny()
	}
	var input struct {
		Bot    string `json:"bot"`
		Prompt string `json:"prompt"`
		Cron   string `json:"cron"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Prompt) == "" || strings.TrimSpace(input.Cron) == "" {
		return "", errors.New("prompt and cron are required")
	}
	target, err := t.store.FindBot(ctx, strings.TrimSpace(input.Bot))
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Routine"
	}
	next, err := scheduleNext(input.Cron)
	if err != nil {
		return "", err
	}
	thread, err := t.store.FirstThread(ctx, target.ID)
	if err != nil {
		return "", err
	}
	routine, err := t.store.CreateRoutine(ctx, target.ID, thread.ID, name, input.Prompt, strings.TrimSpace(input.Cron), next)
	if err != nil {
		return "", err
	}
	return "scheduled " + routine.ID + " on " + target.Name, nil
}

type fleetListTool struct {
	store    *store.Store
	gate     *fleetGate
	memories core.MemoryProvider
}

func (t *fleetListTool) Name() string { return "fleet_list" }

func (t *fleetListTool) Spec() llm.Tool {
	return fleetSpec(t.Name(),
		"List the bots you can see, with one-line status. Needs the fleet switch on.",
		`{"type":"object","properties":{}}`)
}

func (t *fleetListTool) RunFor(ctx context.Context, botID string, _ json.RawMessage) (string, error) {
	if !t.gate.allowed(ctx, botID) {
		return "", t.gate.deny()
	}
	me, err := t.store.GetBot(ctx, botID)
	if err != nil {
		return "", err
	}
	bots, err := t.store.ListBotsIn(ctx, me.SpaceID)
	if err != nil {
		return "", err
	}
	if len(bots) == 0 {
		return "no bots", nil
	}
	lines := make([]string, 0, len(bots))
	for _, bot := range bots {
		marker := ""
		if bot.ID == botID {
			marker = " (you)"
		}
		lines = append(lines, "- "+bot.Name+marker)
	}
	return strings.Join(lines, "\n"), nil
}
