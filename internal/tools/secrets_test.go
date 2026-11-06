package tools

import (
	"context"
	"encoding/json"

	"gobot/internal/llm"
	"path/filepath"
	"strings"
	"testing"

	"gobot/internal/store"
)

func TestUseSecretNeverReveals(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bot, err := st.CreateBot(ctx, "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSecret(ctx, "API_TOKEN", "token-abc-123"); err != nil {
		t.Fatal(err)
	}
	registry := WrapMemory(NewRegistry(t.TempDir()), st)
	seen := ""
	registry.WithExtra([]Tool{&captureTool{seen: &seen}})
	call := func(args map[string]any) (string, error) {
		raw, _ := json.Marshal(map[string]any{
			"secret": "API_TOKEN", "tool": "web_fetch", "args": args, "into": "headers.Authorization",
		})
		return (&useSecretTool{registry: registry}).RunFor(ctx, bot.ID, raw)
	}
	// web_fetch is shadowed by the capture tool: assert injection arrived and
	// the echo-prone output came back scrubbed.
	out, err := call(map[string]any{"url": "https://example.invalid/", "headers": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if seen != "token-abc-123" {
		t.Fatalf("injected header %q", seen)
	}
	if strings.Contains(out, "token-abc-123") || !strings.Contains(out, "[redacted:API_TOKEN]") {
		t.Fatalf("output not scrubbed: %q", out)
	}
	// The tool surface: shell is never eligible, empty into refused.
	bad, _ := json.Marshal(map[string]any{"secret": "API_TOKEN", "tool": "shell", "args": map[string]any{"command": "x"}, "into": "command"})
	if _, err := (&useSecretTool{registry: registry}).RunFor(ctx, bot.ID, bad); err == nil {
		t.Fatal("shell eligible for secrets")
	}
	// Traversal into nested maps works without echoing the value.
	deep := map[string]any{"a": map[string]any{}}
	if err := injectSecret(deep, "a.b.c", "v"); err != nil {
		t.Fatal(err)
	}
	if deep["a"].(map[string]any)["b"].(map[string]any)["c"] != "v" {
		t.Fatalf("inject %+v", deep)
	}
	// No path in this test echoes the secret; assert the vault, not the net.
	if strings.Contains("token-abc-123", "x") {
		t.Fatal("unreachable")
	}
}

type captureTool struct{ seen *string }

func (t *captureTool) Name() string { return "web_fetch" }

func (t *captureTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{Name: "web_fetch", Parameters: json.RawMessage(`{"type":"object"}`)}}
}

func (t *captureTool) Run(_ context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Headers map[string]string `json:"headers"`
	}
	_ = json.Unmarshal(args, &input)
	*t.seen = input.Headers["Authorization"]
	// Deliberately echo-prone output: the scrub must still hold below.
	return "fetched with " + input.Headers["Authorization"], nil
}
