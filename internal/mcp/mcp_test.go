package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			ID     int64  `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05", "serverInfo": map[string]any{"name": "fake", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{
				"name": "echo", "description": "Echo back.",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}},
			}}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "echo-ok"}}, "isError": false}
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		raw, _ := json.Marshal(result)
		out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": json.RawMessage(raw)})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(out)
	}))
}

func TestHTTPListAndCall(t *testing.T) {
	ctx := context.Background()
	server := fakeServer(t)
	defer server.Close()
	client := NewClient(Server{Name: "fake", URL: server.URL})
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools %+v", tools)
	}
	health, err := client.Health(ctx)
	if err != nil || !strings.Contains(health, "fake") {
		t.Fatalf("health %q err %v", health, err)
	}
	out, err := client.CallTool(ctx, "echo", map[string]any{"text": "hi"})
	if err != nil || out != "echo-ok" {
		t.Fatalf("call %q err %v", out, err)
	}
}

func TestHTTPDownServer(t *testing.T) {
	ctx := context.Background()
	client := NewClient(Server{Name: "down", URL: "http://127.0.0.1:1"})
	if _, err := client.ListTools(ctx); err == nil {
		t.Fatal("down server listed")
	}
}

func TestStdioNeedsCommand(t *testing.T) {
	ctx := context.Background()
	client := NewClient(Server{Name: "empty"})
	if _, err := client.ListTools(ctx); err == nil {
		t.Fatal("empty stdio listed")
	}
}
