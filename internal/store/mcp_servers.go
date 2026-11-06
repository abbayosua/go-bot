package store

import (
	"context"
	"encoding/json"
)

// MCPServer is one configured Model Context Protocol server: stdio command
// or streamable HTTP url. Tools surface as mcp_<server>_<tool>.
type MCPServer struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Command   string            `json:"command,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	CreatedAt int64             `json:"createdAt"`
}

const mcpServerSchema = `
CREATE TABLE IF NOT EXISTS mcp_servers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  command TEXT NOT NULL DEFAULT '',
  url TEXT NOT NULL DEFAULT '',
  headers TEXT NOT NULL DEFAULT '{}',
  created_at INTEGER NOT NULL
);
`

// AddMCPServer registers one server: a stdio command or an http url.
func (s *Store) AddMCPServer(ctx context.Context, name, command, url string, headers map[string]string) (MCPServer, error) {
	raw, _ := json.Marshal(headers)
	if string(raw) == "" || string(raw) == "null" {
		raw = []byte("{}")
	}
	server := MCPServer{ID: NewID("mcp"), Name: name, Command: command, URL: url, CreatedAt: now()}
	_ = json.Unmarshal(raw, &server.Headers)
	_, err := s.db.ExecContext(ctx, `INSERT INTO mcp_servers (id, name, command, url, headers, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		server.ID, server.Name, server.Command, server.URL, string(raw), server.CreatedAt)
	return server, err
}

// ListMCPServers returns every registered server.
func (s *Store) ListMCPServers(ctx context.Context) ([]MCPServer, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, command, url, headers, created_at FROM mcp_servers ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MCPServer{}
	for rows.Next() {
		var server MCPServer
		var headers string
		if err := rows.Scan(&server.ID, &server.Name, &server.Command, &server.URL, &headers, &server.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(headers), &server.Headers); err != nil {
			server.Headers = map[string]string{}
		}
		out = append(out, server)
	}
	return out, rows.Err()
}

// DeleteMCPServer removes one server; its tools vanish with it.
func (s *Store) DeleteMCPServer(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM mcp_servers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
