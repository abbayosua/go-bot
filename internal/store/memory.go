package store

import "context"

// Memories, knowledge pages and the shared skill library. Memories carry a
// bot_id, except install-wide shared ones, which use bot_id ''.

const memorySchema = `
CREATE TABLE IF NOT EXISTS memories (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS memories_bot ON memories(bot_id, created_at);

CREATE TABLE IF NOT EXISTS knowledge (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS knowledge_bot ON knowledge(bot_id, created_at);

CREATE TABLE IF NOT EXISTS skills (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
`

type Memory struct {
	ID        string `json:"id"`
	BotID     string `json:"botId"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"createdAt"`
}

type Knowledge struct {
	ID        string `json:"id"`
	BotID     string `json:"botId"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"createdAt"`
}

type Skill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
	CreatedAt   int64  `json:"createdAt"`
}

func (s *Store) AddMemory(ctx context.Context, botID, content string) (Memory, error) {
	memory := Memory{ID: NewID("mem"), BotID: botID, Content: content, CreatedAt: now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO memories (id, bot_id, content, created_at) VALUES (?, ?, ?, ?)`,
		memory.ID, memory.BotID, memory.Content, memory.CreatedAt)
	return memory, err
}

// ListMemories returns the newest entries for a bot, shared install-wide ones
// first so they shape every answer, then the bot's own.
func (s *Store) ListMemories(ctx context.Context, botID string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bot_id, content, created_at FROM memories
		 WHERE bot_id IN ('', ?) ORDER BY created_at DESC, rowid DESC LIMIT ?`, botID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Memory{}
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.BotID, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SearchMemories matches all query words, newest first.
func (s *Store) SearchMemories(ctx context.Context, botID, query string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	words := tokens(query)
	if len(words) == 0 {
		return s.ListMemories(ctx, botID, limit)
	}
	conditions := "WHERE bot_id IN ('', ?)"
	params := []any{botID}
	for _, word := range words {
		conditions += " AND lower(content) LIKE ?"
		params = append(params, "%"+word+"%")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bot_id, content, created_at FROM memories `+conditions+
			` ORDER BY created_at DESC, rowid DESC LIMIT ?`, append(params, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Memory{}
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.BotID, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) DeleteMemory(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id)
	return err
}

func (s *Store) AddKnowledge(ctx context.Context, botID, title, body string) (Knowledge, error) {
	entry := Knowledge{ID: NewID("knw"), BotID: botID, Title: title, Body: body, CreatedAt: now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO knowledge (id, bot_id, title, body, created_at) VALUES (?, ?, ?, ?, ?)`,
		entry.ID, entry.BotID, entry.Title, entry.Body, entry.CreatedAt)
	return entry, err
}

func (s *Store) ListKnowledge(ctx context.Context, botID string) ([]Knowledge, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bot_id, title, body, created_at FROM knowledge
		 WHERE bot_id IN ('', ?) ORDER BY created_at DESC, rowid DESC`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Knowledge{}
	for rows.Next() {
		var k Knowledge
		if err := rows.Scan(&k.ID, &k.BotID, &k.Title, &k.Body, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) SearchKnowledge(ctx context.Context, botID, query string, limit int) ([]Knowledge, error) {
	if limit <= 0 {
		limit = 5
	}
	words := tokens(query)
	conditions := "WHERE bot_id IN ('', ?)"
	params := []any{botID}
	for _, word := range words {
		conditions += " AND (lower(title) LIKE ? OR lower(body) LIKE ?)"
		params = append(params, "%"+word+"%", "%"+word+"%")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bot_id, title, body, created_at FROM knowledge `+conditions+
			` ORDER BY created_at DESC, rowid DESC LIMIT ?`, append(params, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Knowledge{}
	for rows.Next() {
		var k Knowledge
		if err := rows.Scan(&k.ID, &k.BotID, &k.Title, &k.Body, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) DeleteKnowledge(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM knowledge WHERE id = ?`, id)
	return err
}

func (s *Store) CreateSkill(ctx context.Context, name, description, body string) (Skill, error) {
	skill := Skill{ID: NewID("skl"), Name: name, Description: description, Body: body, CreatedAt: now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO skills (id, name, description, body, created_at) VALUES (?, ?, ?, ?, ?)`,
		skill.ID, skill.Name, skill.Description, skill.Body, skill.CreatedAt)
	return skill, err
}

func (s *Store) ListSkills(ctx context.Context) ([]Skill, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, body, created_at FROM skills ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Skill{}
	for rows.Next() {
		var skill Skill
		if err := rows.Scan(&skill.ID, &skill.Name, &skill.Description, &skill.Body, &skill.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, skill)
	}
	return out, rows.Err()
}

func (s *Store) GetSkill(ctx context.Context, name string) (Skill, error) {
	var skill Skill
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, body, created_at FROM skills WHERE name = ?`, name).
		Scan(&skill.ID, &skill.Name, &skill.Description, &skill.Body, &skill.CreatedAt)
	if err != nil {
		return skill, ErrNotFound
	}
	return skill, nil
}

func (s *Store) DeleteSkill(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM skills WHERE id = ?`, id)
	return err
}
