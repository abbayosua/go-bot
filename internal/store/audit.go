package store

import (
	"context"
)

// Audit records who did what: every privileged server action appends one row.
// The log is append-only; there is no update or delete.
type Audit struct {
	ID        string `json:"id"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Target    string `json:"target,omitempty"`
	Detail    string `json:"detail,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

const auditSchema = `
CREATE TABLE IF NOT EXISTS audit (
  id TEXT PRIMARY KEY,
  actor TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  target TEXT NOT NULL DEFAULT '',
  detail TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS audit_time ON audit(created_at DESC);
`

// AddAudit appends one audit row.
func (s *Store) AddAudit(ctx context.Context, actor, action, target, detail string) error {
	if len(detail) > 1000 {
		detail = detail[:1000]
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit (id, actor, action, target, detail, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		NewID("aud"), actor, action, target, detail, now())
	return err
}

// ListAudit returns audit rows newest first, capped at limit.
func (s *Store) ListAudit(ctx context.Context, limit int) ([]Audit, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, actor, action, target, detail, created_at FROM audit ORDER BY created_at DESC, rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Audit{}
	for rows.Next() {
		var a Audit
		if err := rows.Scan(&a.ID, &a.Actor, &a.Action, &a.Target, &a.Detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
