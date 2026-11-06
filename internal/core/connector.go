package core

import "context"

// ConnectorTool is one tool advertisement behind the neutral contract.
type ConnectorTool struct {
	Name        string
	Description string
	Schema      map[string]any
}

// Connector is the neutral contract for external capability sources: MCP
// servers today, a managed catalog tomorrow. Callers never learn which
// adapter answers; adapters differ only in how they dial.
type Connector interface {
	// Name identifies the source in logs, the UI and tool prefixes.
	Name() string
	// Tools lists what the source offers right now.
	Tools(ctx context.Context) ([]ConnectorTool, error)
	// Call runs one tool by name.
	Call(ctx context.Context, tool string, args map[string]any) (string, error)
	// Health is a one-line liveness report for the server list.
	Health(ctx context.Context) (string, error)
}

// Catalog is the optional managed side: a named set of connectors an
// operator curates. Empty by default; presence never changes the API shape,
// only which adapters answer.
type Catalog struct {
	connectors map[string]Connector
}

// NewCatalog builds an empty catalog.
func NewCatalog() *Catalog { return &Catalog{connectors: map[string]Connector{}} }

// Register adds one connector under its own name.
func (c *Catalog) Register(connector Connector) {
	if c.connectors == nil {
		c.connectors = map[string]Connector{}
	}
	c.connectors[connector.Name()] = connector
}

// Lookup finds one connector by name.
func (c *Catalog) Lookup(name string) (Connector, bool) {
	connector, ok := c.connectors[name]
	return connector, ok
}

// Names lists registered connectors, sorted for stable UI.
func (c *Catalog) Names() []string {
	names := make([]string, 0, len(c.connectors))
	for name := range c.connectors {
		names = append(names, name)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}
