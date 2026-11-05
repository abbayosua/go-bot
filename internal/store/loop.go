package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// Feed, instruction versions, approval rules and compaction summaries keep the
// run loop honest, reversible and bounded. Summaries are plain rows: the newest
// summary is the checkpoint, so a resume reuses it instead of rebuilding it.

const loopSchema = `
CREATE TABLE IF NOT EXISTS instruction_versions (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  instructions TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'chat',
  undone INTEGER NOT NULL DEFAULT 0,
  prev_instructions TEXT NOT NULL DEFAULT '',
  seq INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS instruction_versions_bot ON instruction_versions(bot_id, seq DESC);

CREATE TABLE IF NOT EXISTS approval_rules (
  id TEXT PRIMARY KEY,
  scope TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT 'allow',
  tool TEXT NOT NULL DEFAULT '',
  args TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS approval_rules_scope ON approval_rules(scope);

CREATE TABLE IF NOT EXISTS summaries (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  up_to_id TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS summaries_thread ON summaries(thread_id, created_at DESC);
`

type InstructionVersion struct {
	ID               string `json:"id"`
	BotID            string `json:"botId"`
	Instructions     string `json:"instructions"`
	Source           string `json:"source"`
	Undone           bool   `json:"undone"`
	PrevInstructions string `json:"prevInstructions"`
	Seq              int64  `json:"seq"`
	CreatedAt        int64  `json:"createdAt"`
}

type ApprovalRule struct {
	ID        string `json:"id"`
	Scope     string `json:"scope"`
	Kind      string `json:"kind"`
	Tool      string `json:"tool"`
	Args      string `json:"args"`
	CreatedAt int64  `json:"createdAt"`
}

type Summary struct {
	ID        string `json:"id"`
	ThreadID  string `json:"threadId"`
	UpToID    string `json:"upToId"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"createdAt"`
}

const (
	summaryMarker = "[context summary]"
)

// SummaryMarker prefixes the injected summary so the model can tell it from
// fresh conversation.
const SummaryMarker = summaryMarker

// FeedRow is one compact activity row. Call=true carries the request (name and
// summarized arguments); call=false carries the result or refusal.
type FeedRow struct {
	ID      string `json:"id"`
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
	Output  string `json:"output"`
	Call    bool   `json:"call"`
	Error   bool   `json:"error"`
}

// summarizeArgs keeps an activity row to one line: the tool name and the pieces
// whose loss would confuse the reader (paths, commands, queries).
func summarizeArgs(name, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return name
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return name
	}
	parts := []string{name}
	for _, key := range []string{"command", "path", "query", "url", "fact", "name", "prompt"} {
		if value, ok := args[key].(string); ok && strings.TrimSpace(value) != "" {
			parts = append(parts, shorten(value, 80))
		}
	}
	if len(parts) == 1 {
		return name
	}
	return strings.Join(parts, " ")
}

func shorten(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}

// Feed folds one assistant/tool turn into activity rows: a call row for the
// request, then the matching result row.
func Feed(history []Message) []FeedRow {
	var rows []FeedRow
	for _, msg := range history {
		switch msg.Role {
		case "assistant":
			for _, call := range parseToolCalls(msg.ToolCalls) {
				rows = append(rows, FeedRow{
					ID: msg.ID + ":" + call.ID, Tool: call.Name,
					Summary: summarizeArgs(call.Name, call.Arguments), Call: true,
				})
			}
		case "tool":
			rows = append(rows, FeedRow{
				ID: msg.ID, Tool: msg.ToolName, Summary: msg.ToolName + " result",
				Output: msg.Content, Error: strings.HasPrefix(msg.Content, "error:"),
			})
		}
	}
	return rows
}

type storedCall struct {
	ID        string
	Name      string
	Arguments string
}

func parseToolCalls(raw string) []storedCall {
	if raw == "" {
		return nil
	}
	var calls []struct {
		ID       string `json:"id"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal([]byte(raw), &calls); err != nil {
		return nil
	}
	out := make([]storedCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, storedCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
	}
	return out
}

// withTx runs fn in a transaction, rolling back on error.
func (s *Store) withTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AddInstructionVersion(ctx context.Context, botID, instructions, source string) (InstructionVersion, error) {
	version := InstructionVersion{
		ID: NewID("ins"), BotID: botID, Instructions: instructions,
		Source: source, CreatedAt: now(),
	}
	// Snapshot the bot's current instructions at proposal time. Undo restores
	// this value, so the result never depends on timestamp ties or ID order.
	bot, err := s.GetBot(ctx, botID)
	if err != nil {
		return version, err
	}
	version.PrevInstructions = bot.Instructions
	// Monotonic per-bot sequence: insertion order is the source of truth for
	// listing newest-first, independent of created_at ties and random IDs.
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(seq), 0) + 1 FROM instruction_versions WHERE bot_id = ?`, botID).Scan(&version.Seq); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO instruction_versions (id, bot_id, instructions, source, prev_instructions, seq, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			version.ID, version.BotID, version.Instructions, version.Source, version.PrevInstructions, version.Seq, version.CreatedAt)
		return err
	})
	return version, err
}

func (s *Store) ListInstructionVersions(ctx context.Context, botID string, limit int) ([]InstructionVersion, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT id, bot_id, instructions, source, undone, prev_instructions, seq, created_at FROM instruction_versions
		 ORDER BY seq DESC LIMIT ?`
	params := []any{limit}
	if botID != "" {
		query = `SELECT id, bot_id, instructions, source, undone, prev_instructions, seq, created_at FROM instruction_versions
		 WHERE bot_id = ? ORDER BY seq DESC LIMIT ?`
		params = []any{botID, limit}
	}
	rows, err := s.db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InstructionVersion{}
	for rows.Next() {
		var version InstructionVersion
		var undone int
		if err := rows.Scan(&version.ID, &version.BotID, &version.Instructions, &version.Source, &undone, &version.PrevInstructions, &version.Seq, &version.CreatedAt); err != nil {
			return nil, err
		}
		version.Undone = undone != 0
		out = append(out, version)
	}
	return out, rows.Err()
}

// UndoInstructionVersion marks one version undone and reports the bot's
// instructions as they were when the version was proposed. The value is
// captured at proposal time, so undo order is independent of timestamp ties
// and random ID ordering. Listing order uses the monotonic seq column.
func (s *Store) UndoInstructionVersion(ctx context.Context, id string) (string, error) {
	var version InstructionVersion
	var undone int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, bot_id, instructions, undone, prev_instructions FROM instruction_versions WHERE id = ?`, id).
		Scan(&version.ID, &version.BotID, &version.Instructions, &undone, &version.PrevInstructions)
	if err != nil {
		return "", ErrNotFound
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE instruction_versions SET undone = 1 WHERE id = ?`, id); err != nil {
		return "", err
	}
	return version.PrevInstructions, nil
}

func (s *Store) AddApprovalRule(ctx context.Context, scope, kind, tool, args string) (ApprovalRule, error) {
	rule := ApprovalRule{ID: NewID("rul"), Scope: scope, Kind: kind, Tool: tool, Args: args, CreatedAt: now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO approval_rules (id, scope, kind, tool, args, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.Scope, rule.Kind, rule.Tool, rule.Args, rule.CreatedAt)
	return rule, err
}

func (s *Store) ListApprovalRules(ctx context.Context, scope string) ([]ApprovalRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, scope, kind, tool, args, created_at FROM approval_rules
		 WHERE scope IN ('', ?) ORDER BY created_at`, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ApprovalRule{}
	for rows.Next() {
		var rule ApprovalRule
		if err := rows.Scan(&rule.ID, &rule.Scope, &rule.Kind, &rule.Tool, &rule.Args, &rule.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (s *Store) DeleteApprovalRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM approval_rules WHERE id = ?`, id)
	return err
}

func (s *Store) AddSummary(ctx context.Context, threadID, upToID, body string) (Summary, error) {
	summary := Summary{ID: NewID("sum"), ThreadID: threadID, UpToID: upToID, Body: strings.TrimSpace(body), CreatedAt: now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO summaries (id, thread_id, up_to_id, body, created_at) VALUES (?, ?, ?, ?, ?)`,
		summary.ID, summary.ThreadID, summary.UpToID, summary.Body, summary.CreatedAt)
	return summary, err
}

func (s *Store) LatestSummary(ctx context.Context, threadID string) (Summary, error) {
	var summary Summary
	err := s.db.QueryRowContext(ctx,
		`SELECT id, thread_id, up_to_id, body, created_at FROM summaries
		 WHERE thread_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`, threadID).
		Scan(&summary.ID, &summary.ThreadID, &summary.UpToID, &summary.Body, &summary.CreatedAt)
	if err != nil {
		return summary, ErrNotFound
	}
	return summary, nil
}

// CompactHistory keeps the newest rows within the budget and returns
// everything older as dropped. Callers summarise the dropped prefix and
// prepend it as one user message, so the model always sees what came
// before the kept window.
func CompactHistory(history []Message, budget, keep int) (kept []Message, dropped []Message) {
	if chars(history) <= budget {
		return history, nil
	}
	if keep < 1 {
		keep = 1
	}
	if keep >= len(history) {
		return history, nil
	}
	split := len(history) - keep
	return history[split:], history[:split]
}

// SummarizeDropped turns the dropped prefix into paragraphs the model can hold
// in one read: one line per older row, lightly shaped.
func SummarizeDropped(dropped []Message) string {
	var lines []string
	for _, msg := range dropped {
		head := firstLine(msg.body())
		if head == "" {
			continue
		}
		lines = append(lines, rowLabel(msg)+": "+head)
	}
	return strings.Join(lines, "\n")
}

func chars(history []Message) int {
	total := 0
	for _, msg := range history {
		total += len(msg.Content) + len(msg.ToolCalls)
	}
	return total
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	if len(text) > 240 {
		text = text[:240] + "..."
	}
	return text
}

func rowLabel(msg Message) string {
	switch msg.Role {
	case "tool":
		name := msg.ToolName
		if name == "" {
			name = "tool"
		}
		return "tool result (" + name + ")"
	case "assistant":
		if msg.ToolCalls != "" {
			return "assistant tool calls"
		}
		return "assistant"
	default:
		return "user"
	}
}

func (m Message) body() string {
	if m.Role == "tool" {
		if len(m.Content) > 400 {
			return m.Content[:400] + "\n[result truncated in summary]"
		}
		return m.Content
	}
	if m.Role == "assistant" && m.ToolCalls != "" && m.Content == "" {
		names := []string{}
		for _, call := range parseToolCalls(m.ToolCalls) {
			names = append(names, call.Name)
		}
		return "called: " + strings.Join(names, ", ")
	}
	return m.Content
}
