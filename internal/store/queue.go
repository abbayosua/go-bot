package store

import "context"

// QueueStats describes pending work: how many runs wait and how long the
// oldest has waited. The scheduler and the health endpoint read this.
type QueueStats struct {
	Queued      int   `json:"queued"`
	Running     int   `json:"running"`
	OldestWaitS int64 `json:"oldestWaitS"`
}

// Stats returns current queue depth and the oldest queued wait.
func (s *Store) Stats(ctx context.Context) (QueueStats, error) {
	var stats QueueStats
	stats.OldestWaitS = 0
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runs WHERE status = ?`, RunQueued).Scan(&stats.Queued)
	if err != nil {
		return stats, err
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runs WHERE status = ?`, RunRunning).Scan(&stats.Running); err != nil {
		return stats, err
	}
	var oldest *int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT MIN(created_at) FROM runs WHERE status = ?`, RunQueued).Scan(&oldest); err != nil {
		return stats, err
	}
	if oldest != nil {
		wait := now() - *oldest
		if wait > 0 {
			stats.OldestWaitS = wait
		}
	}
	return stats, nil
}
