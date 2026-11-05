package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// Share links expose one thread read-only under an unguessable token.
// Revocation is deletion; there is no expiry clock in this scope.
type Share struct {
	ID        string `json:"id"`
	ThreadID  string `json:"threadId"`
	Token     string `json:"-"`
	CreatedAt int64  `json:"createdAt"`
}

const shareSchema = `
CREATE TABLE IF NOT EXISTS shares (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  token TEXT NOT NULL UNIQUE,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS shares_token ON shares(token);
`

func newShareToken() string {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return NewID("shr")
	}
	return "shr_" + hex.EncodeToString(raw[:])
}

// CreateShare mints a revocable link for a thread.
func (s *Store) CreateShare(ctx context.Context, threadID string) (Share, error) {
	if _, err := s.GetThread(ctx, threadID); err != nil {
		return Share{}, err
	}
	share := Share{ID: NewID("shl"), ThreadID: threadID, Token: newShareToken(), CreatedAt: now()}
	_, err := s.db.ExecContext(ctx, `INSERT INTO shares (id, thread_id, token, created_at) VALUES (?, ?, ?, ?)`,
		share.ID, share.ThreadID, share.Token, share.CreatedAt)
	return share, err
}

// GetShareByToken resolves a link token to its thread.
func (s *Store) GetShareByToken(ctx context.Context, token string) (Share, error) {
	var share Share
	err := s.db.QueryRowContext(ctx, `SELECT id, thread_id, token, created_at FROM shares WHERE token = ?`, token).
		Scan(&share.ID, &share.ThreadID, &share.Token, &share.CreatedAt)
	if err != nil {
		return Share{}, ErrNotFound
	}
	return share, nil
}

// ListShares returns a thread's active links.
func (s *Store) ListShares(ctx context.Context, threadID string) ([]Share, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, thread_id, token, created_at FROM shares WHERE thread_id = ? ORDER BY created_at`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Share{}
	for rows.Next() {
		var share Share
		if err := rows.Scan(&share.ID, &share.ThreadID, &share.Token, &share.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, share)
	}
	return out, rows.Err()
}

// RevokeShare deletes one link.
func (s *Store) RevokeShare(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM shares WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
