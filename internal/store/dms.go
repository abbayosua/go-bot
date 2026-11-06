package store

import (
	"context"
	"errors"
	"strings"
)

var errSelfDM = errors.New("a bot cannot DM itself")

// DM threads are persistent 1:1 conversations between two bots: a shared
// thread owned by the pair, with a readable handoff header. Either bot can
// write; both bots read the same history.
type DM struct {
	ID        string   `json:"id"`
	ThreadID  string   `json:"threadId"`
	MemberIDs []string `json:"memberIds"`
	CreatedAt int64    `json:"createdAt"`
}

const dmSchema = `
CREATE TABLE IF NOT EXISTS dms (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL UNIQUE REFERENCES threads(id) ON DELETE CASCADE,
  member_ids TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
`

// OpenDM returns the shared thread for a bot pair, creating it (with a
// handoff header both bots can read) on first contact.
func (s *Store) OpenDM(ctx context.Context, aID, bID string) (DM, Thread, error) {
	a, err := s.GetBot(ctx, aID)
	if err != nil {
		return DM{}, Thread{}, err
	}
	b, err := s.GetBot(ctx, bID)
	if err != nil {
		return DM{}, Thread{}, err
	}
	if a.ID == b.ID {
		return DM{}, Thread{}, errSelfDM
	}
	first, second := a.ID, b.ID
	if first > second {
		first, second = second, first
	}
	pair := first + "," + second
	var dm DM
	var memberIDs string
	err = s.db.QueryRowContext(ctx, `SELECT id, thread_id, member_ids, created_at FROM dms WHERE member_ids = ?`, pair).
		Scan(&dm.ID, &dm.ThreadID, &memberIDs, &dm.CreatedAt)
	if err == nil {
		dm.MemberIDs = strings.Split(memberIDs, ",")
		thread, terr := s.GetThread(ctx, dm.ThreadID)
		return dm, thread, terr
	}
	thread, err := s.CreateThread(ctx, a.ID, "DM: "+a.Name+" ↔ "+b.Name)
	if err != nil {
		return DM{}, Thread{}, err
	}
	dm = DM{ID: NewID("dm"), ThreadID: thread.ID, MemberIDs: []string{first, second}, CreatedAt: now()}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO dms (id, thread_id, member_ids, created_at) VALUES (?, ?, ?, ?)`,
		dm.ID, dm.ThreadID, pair, dm.CreatedAt); err != nil {
		return DM{}, Thread{}, err
	}
	header := "Direct line between " + a.Name + " and " + b.Name +
		". Anything written here is visible to both bots; quote the other bot by name when you use its words."
	if _, err := s.AddMessage(ctx, Message{ThreadID: thread.ID, Role: "user", Content: header}); err != nil {
		return DM{}, Thread{}, err
	}
	return dm, thread, nil
}

// ListDMs returns every DM thread one bot belongs to, newest first.
func (s *Store) ListDMs(ctx context.Context, botID string) ([]DM, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, thread_id, member_ids, created_at FROM dms WHERE member_ids LIKE '%' || ? || '%' ORDER BY created_at DESC`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DM{}
	for rows.Next() {
		var dm DM
		var memberIDs string
		if err := rows.Scan(&dm.ID, &dm.ThreadID, &memberIDs, &dm.CreatedAt); err != nil {
			return nil, err
		}
		dm.MemberIDs = strings.Split(memberIDs, ",")
		out = append(out, dm)
	}
	return out, rows.Err()
}

// GetDMByThread returns the DM owning a thread, or nil for other threads.
func (s *Store) GetDMByThread(ctx context.Context, threadID string) (*DM, error) {
	var dm DM
	var memberIDs string
	err := s.db.QueryRowContext(ctx, `SELECT id, thread_id, member_ids, created_at FROM dms WHERE thread_id = ?`, threadID).
		Scan(&dm.ID, &dm.ThreadID, &memberIDs, &dm.CreatedAt)
	if err != nil {
		return nil, nil
	}
	dm.MemberIDs = strings.Split(memberIDs, ",")
	return &dm, nil
}

// GetDM returns one DM by id.
func (s *Store) GetDM(ctx context.Context, id string) (DM, error) {
	var dm DM
	var memberIDs string
	err := s.db.QueryRowContext(ctx, `SELECT id, thread_id, member_ids, created_at FROM dms WHERE id = ?`, id).
		Scan(&dm.ID, &dm.ThreadID, &memberIDs, &dm.CreatedAt)
	if err != nil {
		return DM{}, ErrNotFound
	}
	dm.MemberIDs = strings.Split(memberIDs, ",")
	return dm, nil
}
