package store

import (
	"context"
	"database/sql"
	"errors"
)

// FindBot resolves a bot by id or exact name.
func (s *Store) FindBot(ctx context.Context, ref string) (Bot, error) {
	var bot Bot
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, instructions, model, created_at FROM bots WHERE id = ? OR name = ? LIMIT 1`, ref, ref).
		Scan(&bot.ID, &bot.Name, &bot.Instructions, &bot.Model, &bot.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return bot, ErrNotFound
	}
	return bot, err
}
