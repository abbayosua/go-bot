package mcp

import (
	"context"

	"gobot/internal/core"
)

// Connector adapts one MCP server behind the neutral core.Connector
// contract: the agent loop never learns it is talking to MCP.
type Connector struct {
	server Server
}

// NewConnector builds the adapter for one server.
func NewConnector(server Server) *Connector { return &Connector{server: server} }

// Compile-time check: the MCP adapter satisfies the neutral contract.
var _ core.Connector = (*Connector)(nil)

func (c *Connector) Name() string { return c.server.Name }

func (c *Connector) Tools(ctx context.Context) ([]core.ConnectorTool, error) {
	items, err := NewClient(c.server).ListTools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]core.ConnectorTool, 0, len(items))
	for _, item := range items {
		out = append(out, core.ConnectorTool{Name: item.Name, Description: item.Description, Schema: item.Schema})
	}
	return out, nil
}

func (c *Connector) Call(ctx context.Context, tool string, args map[string]any) (string, error) {
	return NewClient(c.server).CallTool(ctx, tool, args)
}

func (c *Connector) Health(ctx context.Context) (string, error) {
	return NewClient(c.server).Health(ctx)
}
