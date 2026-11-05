package store

import "context"

// SetRoutineWatch records the watched path and its current fingerprint.
func (s *Store) SetRoutineWatch(ctx context.Context, id, path, hash string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE routines SET watch_path = ?, watch_hash = ? WHERE id = ?`, path, hash, id)
	return err
}

// WatchedRoutines returns enabled routines that watch a path.
func (s *Store) WatchedRoutines(ctx context.Context) ([]Routine, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bot_id, thread_id, name, prompt, cron, enabled, next_at, last_run_at, last_error, created_at, watch_path, watch_hash
		 FROM routines WHERE enabled = 1 AND watch_path <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Routine{}
	for rows.Next() {
		var r Routine
		var enabled int
		if err := rows.Scan(&r.ID, &r.BotID, &r.ThreadID, &r.Name, &r.Prompt, &r.Cron, &enabled, &r.NextAt, &r.LastRunAt, &r.LastError, &r.CreatedAt, &r.WatchPath, &r.WatchHash); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		out = append(out, r)
	}
	return out, rows.Err()
}
