package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gobot/internal/store"
)

func fakeMCP(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			ID     int64  `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "v", "serverInfo": map[string]any{"name": "fake", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{
				"name": "ping", "description": "Ping.",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			}}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "pong"}}, "isError": false}
		}
		raw, _ := json.Marshal(result)
		out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": json.RawMessage(raw)})
		_, _ = w.Write(out)
	}))
}

func TestBridgeExposesPrefixedTools(t *testing.T) {
	ctx := context.Background()
	server := fakeMCP(t)
	defer server.Close()
	st, err := store.Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.AddMCPServer(ctx, "fake", "", server.URL, nil); err != nil {
		t.Fatal(err)
	}
	bridge := NewMCPBridge(st)
	live, skipped := bridge.LiveTools(ctx)
	if len(skipped) != 0 {
		t.Fatalf("skipped %v", skipped)
	}
	if len(live) != 1 || live[0].Name() != "mcp_fake_ping" {
		t.Fatalf("live tools %+v", live)
	}
	registry := WrapMemory(NewRegistry(t.TempDir()), st).WithExtra(live)
	if !registry.Has("mcp_fake_ping") {
		t.Fatal("bridge tool missing from registry")
	}
	out, err := registry.Run(ctx, "bot", "mcp_fake_ping", json.RawMessage(`{}`))
	if err != nil || out != "pong" {
		t.Fatalf("call %q err %v", out, err)
	}
	found := false
	for _, spec := range registry.Specs() {
		if spec.Function.Name == "mcp_fake_ping" {
			found = true
		}
	}
	if !found {
		t.Fatal("bridge tool missing from specs")
	}
}

func TestBridgeSkipsDeadServers(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.AddMCPServer(ctx, "dead", "", "http://127.0.0.1:1", nil); err != nil {
		t.Fatal(err)
	}
	live, skipped := NewMCPBridge(st).LiveTools(ctx)
	if len(live) != 0 || len(skipped) != 1 {
		t.Fatalf("live %d skipped %v", len(live), skipped)
	}
}
