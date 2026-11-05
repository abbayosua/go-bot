package store

import (
	"context"
	"errors"
)

var errBadStatus = errors.New("status must be todo, doing or done")

// Board cards are the bot's own kanban: todo, doing, done. One board per bot,
// kept out of chat history and memory.
type BoardCard struct {
	ID        string `json:"id"`
	BotID     string `json:"botId"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Position  int    `json:"position"`
	CreatedAt int64  `json:"createdAt"`
}

const boardSchema = `
CREATE TABLE IF NOT EXISTS board_cards (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'todo',
  position INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS board_bot ON board_cards(bot_id, status, position);
`

const (
	BoardTodo  = "todo"
	BoardDoing = "doing"
	BoardDone  = "done"
)

func validBoardStatus(status string) bool {
	return status == BoardTodo || status == BoardDoing || status == BoardDone
}

// AddBoardCard appends a card to a column.
func (s *Store) AddBoardCard(ctx context.Context, botID, title, status string) (BoardCard, error) {
	if status == "" {
		status = BoardTodo
	}
	if !validBoardStatus(status) {
		return BoardCard{}, errBadStatus
	}
	card := BoardCard{ID: NewID("brd"), BotID: botID, Title: title, Status: status, CreatedAt: now()}
	var maxPos *int
	_ = s.db.QueryRowContext(ctx, `SELECT MAX(position) FROM board_cards WHERE bot_id = ? AND status = ?`, botID, status).Scan(&maxPos)
	if maxPos != nil {
		card.Position = *maxPos + 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO board_cards (id, bot_id, title, status, position, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		card.ID, card.BotID, card.Title, card.Status, card.Position, card.CreatedAt)
	return card, err
}

// ListBoardCards returns every card grouped by caller: todo, doing, done.
func (s *Store) ListBoardCards(ctx context.Context, botID string) ([]BoardCard, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bot_id, title, status, position, created_at FROM board_cards WHERE bot_id = ?
		 ORDER BY CASE status WHEN 'todo' THEN 0 WHEN 'doing' THEN 1 ELSE 2 END, position`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BoardCard{}
	for rows.Next() {
		var c BoardCard
		if err := rows.Scan(&c.ID, &c.BotID, &c.Title, &c.Status, &c.Position, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// MoveBoardCard moves a card to another column or reorders within it.
func (s *Store) MoveBoardCard(ctx context.Context, id, status string, position int) (BoardCard, error) {
	card, err := s.GetBoardCard(ctx, id)
	if err != nil {
		return BoardCard{}, err
	}
	if status != "" {
		if !validBoardStatus(status) {
			return BoardCard{}, errBadStatus
		}
		card.Status = status
	}
	card.Position = position
	_, err = s.db.ExecContext(ctx, `UPDATE board_cards SET status = ?, position = ? WHERE id = ?`,
		card.Status, card.Position, id)
	return card, err
}

// GetBoardCard returns one card.
func (s *Store) GetBoardCard(ctx context.Context, id string) (BoardCard, error) {
	var c BoardCard
	err := s.db.QueryRowContext(ctx,
		`SELECT id, bot_id, title, status, position, created_at FROM board_cards WHERE id = ?`, id).
		Scan(&c.ID, &c.BotID, &c.Title, &c.Status, &c.Position, &c.CreatedAt)
	if err != nil {
		return BoardCard{}, ErrNotFound
	}
	return c, nil
}

// DeleteBoardCard removes one card.
func (s *Store) DeleteBoardCard(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM board_cards WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
