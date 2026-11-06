package store

import (
	"context"
)

// Plugin is one installed folder: a manifest on disk plus per-bot scope.
// Enabled plugins add their tools to that bot only; other bots never see them.
type Plugin struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	BotID     string `json:"botId"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"createdAt"`
}

const pluginSchema = `
CREATE TABLE IF NOT EXISTS plugins (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  path TEXT NOT NULL,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL
);
`

// InstallPlugin records one folder for one bot.
func (s *Store) InstallPlugin(ctx context.Context, name, path, botID string) (Plugin, error) {
	if _, err := s.GetBot(ctx, botID); err != nil {
		return Plugin{}, err
	}
	plugin := Plugin{ID: NewID("plg"), Name: name, Path: path, BotID: botID, Enabled: true, CreatedAt: now()}
	_, err := s.db.ExecContext(ctx, `INSERT INTO plugins (id, name, path, bot_id, enabled, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		plugin.ID, plugin.Name, plugin.Path, plugin.BotID, 1, plugin.CreatedAt)
	return plugin, err
}

// ListPlugins returns one bot's plugins.
func (s *Store) ListPlugins(ctx context.Context, botID string) ([]Plugin, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, path, bot_id, enabled, created_at FROM plugins WHERE bot_id = ? ORDER BY created_at`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plugin{}
	for rows.Next() {
		var plugin Plugin
		var enabled int
		if err := rows.Scan(&plugin.ID, &plugin.Name, &plugin.Path, &plugin.BotID, &enabled, &plugin.CreatedAt); err != nil {
			return nil, err
		}
		plugin.Enabled = enabled != 0
		out = append(out, plugin)
	}
	return out, rows.Err()
}

// SetPluginEnabled flips one plugin.
func (s *Store) SetPluginEnabled(ctx context.Context, id string, enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE plugins SET enabled = ? WHERE id = ?`, value, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePlugin uninstalls one plugin.
func (s *Store) DeletePlugin(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM plugins WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
