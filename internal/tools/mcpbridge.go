package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gobot/internal/llm"
	"gobot/internal/mcp"
	"gobot/internal/store"
)

// mcpTool exposes one connector tool as mcp_<source>_<tool>. Calls run live
// through the neutral core.Connector contract; approval policy sees the
// full prefixed name, so a rule can allow or deny per source.
type mcpTool struct {
	source string
	name   string
	schema map[string]any
	desc   string
	call   func(ctx context.Context, args map[string]any) (string, error)
}

func mcpName(server, tool string) string {
	clean := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
				return r
			}
			return '_'
		}, s)
	}
	return "mcp_" + clean(server) + "_" + clean(tool)
}

func (t *mcpTool) Name() string { return mcpName(t.source, t.name) }

func (t *mcpTool) Spec() llm.Tool {
	schema := t.schema
	if len(schema) == 0 {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	raw, _ := json.Marshal(schema)
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        t.Name(),
		Description: "MCP " + t.source + "." + t.name + ": " + t.desc,
		Parameters:  json.RawMessage(raw),
	}}
}

func (t *mcpTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
	var params map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return "", err
		}
	}
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return t.call(callCtx, params)
}

// MCPBridge lists live connector tools for one registry root. Sources are
// core.Connector adapters (MCP servers today, a managed catalog tomorrow);
// the loop below never learns which kind answers.
type MCPBridge struct {
	store *store.Store
}

func NewMCPBridge(st *store.Store) *MCPBridge { return &MCPBridge{store: st} }

// LiveTools dials every registered source, lists its tools and returns them
// prefixed. A dead source is skipped with its name recorded, never fatal.
func (b *MCPBridge) LiveTools(ctx context.Context) ([]Tool, []string) {
	servers, err := b.store.ListMCPServers(ctx)
	if err != nil {
		return nil, []string{fmt.Sprintf("mcp list: %v", err)}
	}
	var out []Tool
	var skipped []string
	for _, server := range servers {
		connector := mcp.NewConnector(mcp.Server{Name: server.Name, Command: server.Command, URL: server.URL, Headers: server.Headers})
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		items, err := connector.Tools(callCtx)
		cancel()
		if err != nil {
			skipped = append(skipped, server.Name+": "+err.Error())
			continue
		}
		for _, item := range items {
			item := item
			name := item.Name
			out = append(out, &mcpTool{
				source: connector.Name(),
				name:   name,
				schema: item.Schema,
				desc:   item.Description,
				call: func(callCtx context.Context, params map[string]any) (string, error) {
					return connector.Call(callCtx, name, params)
				},
			})
		}
	}
	return out, skipped
}
