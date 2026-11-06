package store

import (
	"context"
)

// ExternalAgent is one outside agent service a bot may hand work to: an
// https endpoint that takes {task, callback} and later POSTs {token, result}
// back. Results read back like a subagent's reply.
type ExternalAgent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	CreatedAt int64  `json:"createdAt"`
}

const externalAgentSchema = `
CREATE TABLE IF NOT EXISTS external_agents (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  url TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS external_tasks (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL REFERENCES external_agents(id) ON DELETE CASCADE,
  run_id TEXT NOT NULL DEFAULT '',
  token TEXT NOT NULL UNIQUE,
  task TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  result TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
`

// AddExternalAgent registers one outside service.
func (s *Store) AddExternalAgent(ctx context.Context, name, url string) (ExternalAgent, error) {
	agent := ExternalAgent{ID: NewID("xag"), Name: name, URL: url, CreatedAt: now()}
	_, err := s.db.ExecContext(ctx, `INSERT INTO external_agents (id, name, url, created_at) VALUES (?, ?, ?, ?)`,
		agent.ID, agent.Name, agent.URL, agent.CreatedAt)
	return agent, err
}

// ListExternalAgents returns every registered service.
func (s *Store) ListExternalAgents(ctx context.Context) ([]ExternalAgent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, url, created_at FROM external_agents ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExternalAgent{}
	for rows.Next() {
		var agent ExternalAgent
		if err := rows.Scan(&agent.ID, &agent.Name, &agent.URL, &agent.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, agent)
	}
	return out, rows.Err()
}

// FindExternalAgent resolves one service by id or name.
func (s *Store) FindExternalAgent(ctx context.Context, ref string) (ExternalAgent, error) {
	var agent ExternalAgent
	err := s.db.QueryRowContext(ctx, `SELECT id, name, url, created_at FROM external_agents WHERE id = ? OR name = ? LIMIT 1`, ref, ref).
		Scan(&agent.ID, &agent.Name, &agent.URL, &agent.CreatedAt)
	if err != nil {
		return ExternalAgent{}, ErrNotFound
	}
	return agent, nil
}

// DeleteExternalAgent removes one service and its task rows.
func (s *Store) DeleteExternalAgent(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM external_agents WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ExternalTask tracks one hand-off: pending until the callback lands.
type ExternalTask struct {
	ID        string
	AgentID   string
	RunID     string
	Token     string
	Task      string
	Status    string
	Result    string
	CreatedAt int64
}

// CreateExternalTask records a hand-off and mints its callback token.
func (s *Store) CreateExternalTask(ctx context.Context, agentID, runID, task string) (ExternalTask, error) {
	out := ExternalTask{ID: NewID("xtk"), AgentID: agentID, RunID: runID, Token: newShareToken(), Task: task, Status: "pending", CreatedAt: now()}
	_, err := s.db.ExecContext(ctx, `INSERT INTO external_tasks (id, agent_id, run_id, token, task, status, result, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		out.ID, out.AgentID, out.RunID, out.Token, out.Task, out.Status, "", out.CreatedAt)
	return out, err
}

// CompleteExternalTask stores a callback result; unknown tokens are refused.
func (s *Store) CompleteExternalTask(ctx context.Context, token, result string) (ExternalTask, error) {
	var task ExternalTask
	err := s.db.QueryRowContext(ctx, `SELECT id, agent_id, run_id, token, task, status, result FROM external_tasks WHERE token = ?`, token).
		Scan(&task.ID, &task.AgentID, &task.RunID, &task.Token, &task.Task, &task.Status, &task.Result)
	if err != nil {
		return ExternalTask{}, ErrNotFound
	}
	if len(result) > 8000 {
		result = result[:8000]
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE external_tasks SET status = 'done', result = ? WHERE id = ?`, result, task.ID); err != nil {
		return ExternalTask{}, err
	}
	task.Status, task.Result = "done", result
	return task, nil
}

// ExternalTaskResult reads a finished hand-off for the waiting run.
func (s *Store) ExternalTaskResult(ctx context.Context, runID, taskID string) (ExternalTask, error) {
	var task ExternalTask
	err := s.db.QueryRowContext(ctx, `SELECT id, agent_id, run_id, token, task, status, result FROM external_tasks WHERE id = ? AND run_id = ?`, taskID, runID).
		Scan(&task.ID, &task.AgentID, &task.RunID, &task.Token, &task.Task, &task.Status, &task.Result)
	if err != nil {
		return ExternalTask{}, ErrNotFound
	}
	return task, nil
}
