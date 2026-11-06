package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gobot/internal/llm"
)

// useSecretTool lets the model spend a secret without ever seeing it: the
// vault injects the value into one argument of a whitelisted tool call, runs
// it, then scrubs the value from the output. Shell is never eligible:
// secrets flow only to structured tools (web_fetch headers today).
type useSecretTool struct {
	registry *MemoryRegistry
}

func (t *useSecretTool) Name() string { return "use_secret" }

func (t *useSecretTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "use_secret",
		Description: "Spend a named secret inside one tool call without seeing it. The secret is injected into one argument, the tool runs, and the value is scrubbed from the result. Ask the user first if the secret name is unknown.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"secret": {"type": "string", "description": "Secret name, e.g. GITHUB_TOKEN."},
				"tool": {"type": "string", "description": "Tool to run: web_fetch only."},
				"args": {"type": "object", "description": "Tool arguments with {{secret}} where the value goes."},
				"into": {"type": "string", "description": "Argument path receiving the secret, e.g. headers.Authorization."}
			},
			"required": ["secret", "tool", "args", "into"]
		}`),
	}}
}

var secretEligibleTools = map[string]bool{"web_fetch": true}

func (t *useSecretTool) RunFor(ctx context.Context, botID string, args json.RawMessage) (string, error) {
	var input struct {
		Secret string         `json:"secret"`
		Tool   string         `json:"tool"`
		Args   map[string]any `json:"args"`
		Into   string         `json:"into"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Secret) == "" || !secretEligibleTools[input.Tool] || len(input.Args) == 0 || strings.TrimSpace(input.Into) == "" {
		return "", errors.New("secret, eligible tool (web_fetch), args and into are required")
	}
	registry := t.registry
	if registry == nil || registry.store == nil {
		return "", errors.New("secret vault is not wired for this run")
	}
	if !registry.store.HasSecret(ctx, input.Secret) {
		return "", errors.New("unknown secret (ask the user to add it in settings)")
	}
	return registry.store.UseSecret(ctx, strings.TrimSpace(input.Secret), func(value string) (string, error) {
		if err := injectSecret(input.Args, input.Into, value); err != nil {
			return "", err
		}
		raw, err := json.Marshal(input.Args)
		if err != nil {
			return "", err
		}
		out, err := registry.Run(ctx, botID, input.Tool, raw)
		redacted := scrubSecret(out, input.Secret, value)
		value = ""
		if err != nil {
			return "", err
		}
		return redacted, nil
	})
}

// scrubSecret redacts the value everywhere it leaks into tool output, so the
// model only ever sees the secret name, never the bytes.
func scrubSecret(out, name, value string) string {
	if value == "" {
		return out
	}
	redacted := strings.ReplaceAll(out, value, "[redacted:"+name+"]")
	return redacted
}

// injectSecret sets args[a.b.c] to the secret value.
func injectSecret(args map[string]any, path, value string) error {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || parts[0] == "" {
		return errors.New("into path is empty")
	}
	node := args
	for _, part := range parts[:len(parts)-1] {
		next, ok := node[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			node[part] = next
		}
		node = next
	}
	node[parts[len(parts)-1]] = value
	return nil
}
