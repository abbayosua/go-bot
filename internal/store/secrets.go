package store

import (
	"context"
	"errors"
	"strings"
)

// Secret metadata: the value lives encrypted in secret_blobs. Plaintext never
// touches the database file, tool output, or logs: tools receive secrets by
// injection at call time, and the model only ever sees names.
type Secret struct {
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
}

const secretSchema = `
CREATE TABLE IF NOT EXISTS secrets (
  name TEXT PRIMARY KEY,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS secret_blobs (
  name TEXT PRIMARY KEY REFERENCES secrets(name) ON DELETE CASCADE,
  nonce BLOB NOT NULL,
  sealed BLOB NOT NULL
);
`

// SetSecret stores one named secret, encrypted at rest.
func (s *Store) SetSecret(ctx context.Context, name, value string) error {
	name = strings.TrimSpace(name)
	if name == "" || value == "" {
		return errors.New("name and value are required")
	}
	if len(value) > 8<<10 {
		return errors.New("secret over 8 KiB")
	}
	salt, err := s.SessionSalt(ctx)
	if err != nil {
		return err
	}
	nonce, sealed, err := sealAttachment(deriveKey("secret-vault:"+name, salt), []byte(value))
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO secrets (name, created_at) VALUES (?, ?) ON CONFLICT(name) DO NOTHING`, name, now()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO secret_blobs (name, nonce, sealed) VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET nonce = excluded.nonce, sealed = excluded.sealed`, name, nonce, sealed); err != nil {
		return err
	}
	return tx.Commit()
}

// HasSecret reports whether a name exists, without revealing anything else.
func (s *Store) HasSecret(ctx context.Context, name string) bool {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM secrets WHERE name = ?`, strings.TrimSpace(name)).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

// ListSecrets returns secret names only, newest last.
func (s *Store) ListSecrets(ctx context.Context) ([]Secret, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, created_at FROM secrets ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Secret{}
	for rows.Next() {
		var secret Secret
		if err := rows.Scan(&secret.Name, &secret.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, secret)
	}
	return out, rows.Err()
}

// DeleteSecret removes one named secret and its blob.
func (s *Store) DeleteSecret(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM secrets WHERE name = ?`, strings.TrimSpace(name))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// revealSecret decrypts one value for internal tool injection. It is
// deliberately unexported: only the tools package may call useSecret.
func (s *Store) revealSecret(ctx context.Context, name string) (string, error) {
	var nonce, sealed []byte
	err := s.db.QueryRowContext(ctx, `SELECT nonce, sealed FROM secret_blobs WHERE name = ?`, strings.TrimSpace(name)).
		Scan(&nonce, &sealed)
	if err != nil {
		return "", ErrNotFound
	}
	salt, err := s.SessionSalt(ctx)
	if err != nil {
		return "", err
	}
	plaintext, err := openAttachment(deriveKey("secret-vault:"+strings.TrimSpace(name), salt), nonce, sealed)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// UseSecret runs fn with the decrypted value, then drops it. The plaintext is
// never returned, logged, or stored: it only lives inside fn.
func (s *Store) UseSecret(ctx context.Context, name string, fn func(value string) (string, error)) (string, error) {
	value, err := s.revealSecret(ctx, name)
	if err != nil {
		return "", err
	}
	out, err := fn(value)
	value = ""
	return out, err
}
