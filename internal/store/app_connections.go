package store

import (
	"context"
	"errors"
	"strings"
)

var errAppConn = errors.New("name and access token are required")

// AppConnection is one OAuth-style connection: the access token lives
// encrypted in the secrets vault under "oauth:<name>", the refresh token
// beside it; the row only carries metadata. Refresh posts to the token URL
// and stores the new access token without user action.
type AppConnection struct {
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	TokenURL   string `json:"tokenUrl,omitempty"`
	ExpiresAt  int64  `json:"expiresAt,omitempty"`
	CreatedAt  int64  `json:"createdAt"`
	HasRefresh bool   `json:"hasRefresh"`
}

const appConnSchema = `
CREATE TABLE IF NOT EXISTS app_connections (
  name TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  provider TEXT NOT NULL DEFAULT '',
  token_url TEXT NOT NULL DEFAULT '',
  expires_at INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
`

func secretFor(name string) string { return "oauth:" + strings.TrimSpace(name) }

// ConnectApp stores a connection: tokens go to the vault, metadata to the row.
func (s *Store) ConnectApp(ctx context.Context, botID, name, provider, access, refresh, tokenURL string, expiresAt int64) (AppConnection, error) {
	name = strings.TrimSpace(name)
	if name == "" || access == "" {
		return AppConnection{}, errAppConn
	}
	if _, err := s.GetBot(ctx, botID); err != nil {
		return AppConnection{}, err
	}
	if err := s.SetSecret(ctx, secretFor(name), access); err != nil {
		return AppConnection{}, err
	}
	if strings.TrimSpace(refresh) != "" {
		if err := s.SetSecret(ctx, secretFor(name)+":refresh", refresh); err != nil {
			return AppConnection{}, err
		}
	}
	conn := AppConnection{Name: name, Provider: provider, TokenURL: tokenURL, ExpiresAt: expiresAt, CreatedAt: now(), HasRefresh: strings.TrimSpace(refresh) != ""}
	_, err := s.db.ExecContext(ctx, `INSERT INTO app_connections (name, bot_id, provider, token_url, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET provider = excluded.provider, token_url = excluded.token_url, expires_at = excluded.expires_at`,
		name, botID, provider, tokenURL, expiresAt, conn.CreatedAt)
	return conn, err
}

// ListAppConnections returns one bot's connections (tokens never leave).
func (s *Store) ListAppConnections(ctx context.Context, botID string) ([]AppConnection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, provider, token_url, expires_at, created_at FROM app_connections WHERE bot_id = ? ORDER BY created_at`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AppConnection{}
	for rows.Next() {
		var conn AppConnection
		if err := rows.Scan(&conn.Name, &conn.Provider, &conn.TokenURL, &conn.ExpiresAt, &conn.CreatedAt); err != nil {
			return nil, err
		}
		conn.HasRefresh = s.HasSecret(ctx, secretFor(conn.Name)+":refresh")
		out = append(out, conn)
	}
	return out, rows.Err()
}

// DeleteAppConnection removes a connection and its vault tokens.
func (s *Store) DeleteAppConnection(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM app_connections WHERE name = ?`, strings.TrimSpace(name))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_ = s.DeleteSecret(ctx, secretFor(name))
	_ = s.DeleteSecret(ctx, secretFor(name)+":refresh")
	return nil
}

// AppAccessToken returns the current access token for internal callers.
func (s *Store) AppAccessToken(ctx context.Context, name string) (string, error) {
	return s.revealSecret(ctx, secretFor(name))
}

// AppRefreshToken returns the stored refresh token for internal callers.
func (s *Store) AppRefreshToken(ctx context.Context, name string) (string, error) {
	return s.revealSecret(ctx, secretFor(name)+":refresh")
}

// FindAppConnection returns one connection row by name.
func (s *Store) FindAppConnection(ctx context.Context, name string) (AppConnection, string, error) {
	var conn AppConnection
	var botID string
	err := s.db.QueryRowContext(ctx, `SELECT name, bot_id, provider, token_url, expires_at, created_at FROM app_connections WHERE name = ?`, name).
		Scan(&conn.Name, &botID, &conn.Provider, &conn.TokenURL, &conn.ExpiresAt, &conn.CreatedAt)
	if err != nil {
		return AppConnection{}, "", ErrNotFound
	}
	conn.HasRefresh = s.HasSecret(ctx, secretFor(name)+":refresh")
	return conn, botID, nil
}
