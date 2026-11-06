package store

import (
	"context"
	"strings"
)

// MsgInstall connects one bot to one messaging adapter: the bot token for
// outbound, plus a share list of chat ids allowed to reach the bot inbound.
// Removing a chat stops its messages; removing the install stops everything.
type MsgInstall struct {
	ID        string `json:"id"`
	BotID     string `json:"botId"`
	Adapter   string `json:"adapter"`
	Token     string `json:"-"`
	Chats     string `json:"chats"`
	Thread    string `json:"threadId,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

const msgSchema = `
CREATE TABLE IF NOT EXISTS msg_installs (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  adapter TEXT NOT NULL DEFAULT 'telegram',
  token TEXT NOT NULL DEFAULT '',
  chats TEXT NOT NULL DEFAULT '',
  thread_id TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
`

// AddMsgInstall connects a bot to an adapter with an initial share list.
func (s *Store) AddMsgInstall(ctx context.Context, botID, adapter, token, chats, threadID string) (MsgInstall, error) {
	if _, err := s.GetBot(ctx, botID); err != nil {
		return MsgInstall{}, err
	}
	install := MsgInstall{
		ID: NewID("msg"), BotID: botID, Adapter: adapter, Token: token,
		Chats: normalizeChats(chats), Thread: threadID, CreatedAt: now(),
	}
	if install.Adapter == "" {
		install.Adapter = "telegram"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO msg_installs (id, bot_id, adapter, token, chats, thread_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		install.ID, install.BotID, install.Adapter, install.Token, install.Chats, install.Thread, install.CreatedAt)
	install.Token = ""
	return install, err
}

// ListMsgInstalls returns one bot's connections (tokens never leave).
func (s *Store) ListMsgInstalls(ctx context.Context, botID string) ([]MsgInstall, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, bot_id, adapter, chats, thread_id, created_at FROM msg_installs WHERE bot_id = ? ORDER BY created_at`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MsgInstall{}
	for rows.Next() {
		var install MsgInstall
		if err := rows.Scan(&install.ID, &install.BotID, &install.Adapter, &install.Chats, &install.Thread, &install.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, install)
	}
	return out, rows.Err()
}

// SetMsgChats replaces the share list: who may talk to the bot.
func (s *Store) SetMsgChats(ctx context.Context, id, chats string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE msg_installs SET chats = ? WHERE id = ?`, normalizeChats(chats), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteMsgInstall removes one connection.
func (s *Store) DeleteMsgInstall(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM msg_installs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MsgInstallForToken resolves an inbound webhook token to its install.
func (s *Store) MsgInstallForToken(ctx context.Context, token string) (MsgInstall, string, error) {
	var install MsgInstall
	err := s.db.QueryRowContext(ctx, `SELECT id, bot_id, adapter, token, chats, thread_id, created_at FROM msg_installs WHERE token = ?`, token).
		Scan(&install.ID, &install.BotID, &install.Adapter, &install.Token, &install.Chats, &install.Thread, &install.CreatedAt)
	if err != nil {
		return MsgInstall{}, "", ErrNotFound
	}
	raw := install.Token
	install.Token = ""
	return install, raw, nil
}

// ChatAllowed reports whether a chat id is on the share list. An empty list
// means nobody: the operator adds chats explicitly.
func ChatAllowed(list, chat string) bool {
	chat = strings.TrimSpace(chat)
	if chat == "" {
		return false
	}
	for _, entry := range strings.Split(list, ",") {
		if strings.TrimSpace(entry) == chat {
			return true
		}
	}
	return false
}

func normalizeChats(list string) string {
	seen := map[string]bool{}
	var out []string
	for _, entry := range strings.Split(list, ",") {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return strings.Join(out, ",")
}
