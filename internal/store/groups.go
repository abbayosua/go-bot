package store

import (
	"context"
	"errors"
	"strings"
)

var errGroupSize = errors.New("groups hold 2-10 bots")

var errGroupSpace = errors.New("every member must live in the group space")

// Group bounds one shared chat: 2-10 member bots, each keeping its own memory
// and workspace. The thread stays the conversation; the group row lists who
// is in it. Mentions (@Name) in a user message decide whose turn it is.
type Group struct {
	ID        string   `json:"id"`
	ThreadID  string   `json:"threadId"`
	SpaceID   string   `json:"spaceId"`
	MemberIDs []string `json:"memberIds"`
	CreatedAt int64    `json:"createdAt"`
}

const groupSchema = `
CREATE TABLE IF NOT EXISTS groups (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL UNIQUE REFERENCES threads(id) ON DELETE CASCADE,
  space_id TEXT NOT NULL DEFAULT '',
  member_ids TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
`

const (
	GroupMinMembers = 2
	GroupMaxMembers = 10
)

// CreateGroup starts a shared chat for 2-10 bots in one space. It creates the
// thread itself so the thread can never escape the group.
func (s *Store) CreateGroup(ctx context.Context, spaceID string, memberIDs []string, title string) (Group, Thread, error) {
	seen := map[string]bool{}
	members := make([]string, 0, len(memberIDs))
	for _, id := range memberIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		members = append(members, id)
	}
	if len(members) < GroupMinMembers {
		return Group{}, Thread{}, errGroupSize
	}
	if len(members) > GroupMaxMembers {
		return Group{}, Thread{}, errGroupSize
	}
	space := spaceID
	if space == "" {
		defaultSpace, err := s.DefaultSpace(ctx)
		if err != nil {
			return Group{}, Thread{}, err
		}
		space = defaultSpace.ID
	}
	for _, id := range members {
		bot, err := s.GetBot(ctx, id)
		if err != nil {
			return Group{}, Thread{}, err
		}
		if bot.SpaceID != space {
			return Group{}, Thread{}, errGroupSpace
		}
	}
	thread, err := s.CreateThread(ctx, members[0], title)
	if err != nil {
		return Group{}, Thread{}, err
	}
	group := Group{ID: NewID("grp"), ThreadID: thread.ID, SpaceID: space, MemberIDs: members, CreatedAt: now()}
	_, err = s.db.ExecContext(ctx, `INSERT INTO groups (id, thread_id, space_id, member_ids, created_at) VALUES (?, ?, ?, ?, ?)`,
		group.ID, group.ThreadID, group.SpaceID, strings.Join(members, ","), group.CreatedAt)
	return group, thread, err
}

// GetGroupByThread returns the group owning a thread, or nil for 1:1 chats.
func (s *Store) GetGroupByThread(ctx context.Context, threadID string) (*Group, error) {
	var group Group
	var memberIDs string
	err := s.db.QueryRowContext(ctx, `SELECT id, thread_id, space_id, member_ids, created_at FROM groups WHERE thread_id = ?`, threadID).
		Scan(&group.ID, &group.ThreadID, &group.SpaceID, &memberIDs, &group.CreatedAt)
	if err != nil {
		return nil, nil
	}
	if memberIDs != "" {
		group.MemberIDs = strings.Split(memberIDs, ",")
	}
	return &group, nil
}

// ListGroups returns one space's groups, newest last.
func (s *Store) ListGroups(ctx context.Context, spaceID string) ([]Group, error) {
	query := `SELECT id, thread_id, space_id, member_ids, created_at FROM groups ORDER BY created_at`
	params := []any{}
	if spaceID != "" {
		query = `SELECT id, thread_id, space_id, member_ids, created_at FROM groups WHERE space_id = ? ORDER BY created_at`
		params = append(params, spaceID)
	}
	rows, err := s.db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var group Group
		var memberIDs string
		if err := rows.Scan(&group.ID, &group.ThreadID, &group.SpaceID, &memberIDs, &group.CreatedAt); err != nil {
			return nil, err
		}
		if memberIDs != "" {
			group.MemberIDs = strings.Split(memberIDs, ",")
		}
		out = append(out, group)
	}
	return out, rows.Err()
}
