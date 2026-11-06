package store

import (
	"context"
	"errors"
)

var errSpaceNotEmpty = errors.New("space still holds bots")

// Space owns bots, threads, memories and a workspace root. Every bot lives in
// exactly one space; threads and memories follow their bot. The default space
// keeps single-user installs working unchanged.
type Space struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
}

const spaceSchema = `
CREATE TABLE IF NOT EXISTS spaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
`

const defaultSpaceName = "Personal"

// DefaultSpace returns the personal space, creating it on first use.
func (s *Store) DefaultSpace(ctx context.Context) (Space, error) {
	var space Space
	err := s.db.QueryRowContext(ctx, `SELECT id, name, created_at FROM spaces ORDER BY created_at LIMIT 1`).
		Scan(&space.ID, &space.Name, &space.CreatedAt)
	if err == nil {
		return space, nil
	}
	space = Space{ID: NewID("spc"), Name: defaultSpaceName, CreatedAt: now()}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO spaces (id, name, created_at) VALUES (?, ?, ?)`,
		space.ID, space.Name, space.CreatedAt); err != nil {
		return Space{}, err
	}
	return space, nil
}

// CreateSpace makes one named space.
func (s *Store) CreateSpace(ctx context.Context, name string) (Space, error) {
	space := Space{ID: NewID("spc"), Name: name, CreatedAt: now()}
	_, err := s.db.ExecContext(ctx, `INSERT INTO spaces (id, name, created_at) VALUES (?, ?, ?)`,
		space.ID, space.Name, space.CreatedAt)
	return space, err
}

// ListSpaces returns every space, oldest first.
func (s *Store) ListSpaces(ctx context.Context) ([]Space, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, created_at FROM spaces ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Space{}
	for rows.Next() {
		var space Space
		if err := rows.Scan(&space.ID, &space.Name, &space.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, space)
	}
	return out, rows.Err()
}

// DeleteSpace removes an empty space. Spaces holding bots are refused so bots
// are never orphaned by a stray click.
func (s *Store) DeleteSpace(ctx context.Context, id string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bots WHERE space_id = ?`, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return errSpaceNotEmpty
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM spaces WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListBotsIn lists one space's bots.
func (s *Store) ListBotsIn(ctx context.Context, spaceID string) ([]Bot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, instructions, model, space_id, created_at FROM bots WHERE space_id = ? ORDER BY created_at`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Bot{}
	for rows.Next() {
		var bot Bot
		if err := rows.Scan(&bot.ID, &bot.Name, &bot.Instructions, &bot.Model, &bot.SpaceID, &bot.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, bot)
	}
	return out, rows.Err()
}
