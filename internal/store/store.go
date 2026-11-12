package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS bots (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  instructions TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  space_id TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS threads (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  title TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS threads_bot ON threads(bot_id, created_at DESC);

CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '',
  tool_calls TEXT NOT NULL DEFAULT '',
  tool_call_id TEXT NOT NULL DEFAULT '',
  tool_name TEXT NOT NULL DEFAULT '',
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  cached_tokens INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS messages_thread ON messages(thread_id, created_at, id);

CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  thread_id TEXT NOT NULL,
  trigger TEXT NOT NULL DEFAULT 'user',
  prompt TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'queued',
  error TEXT NOT NULL DEFAULT '',
  steps INTEGER NOT NULL DEFAULT 0,
  lease_until INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  started_at INTEGER NOT NULL DEFAULT 0,
  finished_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS runs_thread ON runs(thread_id, created_at DESC);

CREATE TABLE IF NOT EXISTS approvals (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  thread_id TEXT NOT NULL,
  tool_call_id TEXT NOT NULL,
  tool TEXT NOT NULL,
  args TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  created_at INTEGER NOT NULL,
  decided_at INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS approvals_call ON approvals(run_id, tool_call_id);
CREATE INDEX IF NOT EXISTS approvals_thread ON approvals(thread_id, status);

CREATE TABLE IF NOT EXISTS routines (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  thread_id TEXT NOT NULL,
  name TEXT NOT NULL,
  prompt TEXT NOT NULL,
  cron TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  next_at INTEGER NOT NULL DEFAULT 0,
  last_run_at INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  watch_path TEXT NOT NULL DEFAULT '',
  watch_hash TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS routines_due ON routines(enabled, next_at);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`

const (
	RunQueued          = "queued"
	RunRunning         = "running"
	RunWaitingApproval = "waiting_approval"
	RunDone            = "done"
	RunFailed          = "failed"
	RunCancelled       = "cancelled"
)

const (
	// Stored approval rows only ever hold Pending, Allow or Deny.
	ApprovalPending = "pending"
	ApprovalAllow   = "allow"
	ApprovalDeny    = "deny"
	// ApprovalAsk is a gate outcome, never stored: the tool needs a person.
	ApprovalAsk = "ask"
)

type Bot struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Instructions string `json:"instructions"`
	Model        string `json:"model"`
	SpaceID      string `json:"spaceId,omitempty"`
	CreatedAt    int64  `json:"createdAt"`
}

type Thread struct {
	ID        string `json:"id"`
	BotID     string `json:"botId"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"createdAt"`
}

type Message struct {
	ID           string `json:"id"`
	ThreadID     string `json:"threadId"`
	Role         string `json:"role"`
	Content      string `json:"content"`
	ToolCalls    string `json:"toolCalls,omitempty"`
	ToolCallID   string `json:"toolCallId,omitempty"`
	ToolName     string `json:"toolName,omitempty"`
	PromptTokens int    `json:"promptTokens,omitempty"`
	CachedTokens int    `json:"cachedTokens,omitempty"`
	CreatedAt    int64  `json:"createdAt"`
}

type Run struct {
	ID         string `json:"id"`
	BotID      string `json:"botId"`
	ThreadID   string `json:"threadId"`
	Trigger    string `json:"trigger"`
	Prompt     string `json:"prompt"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	Steps      int    `json:"steps"`
	LeaseUntil int64  `json:"-"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
	StartedAt  int64  `json:"startedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
	QueueWaitS int64  `json:"queueWaitS,omitempty"`
}

type Approval struct {
	ID         string `json:"id"`
	RunID      string `json:"runId"`
	ThreadID   string `json:"threadId"`
	ToolCallID string `json:"toolCallId"`
	Tool       string `json:"tool"`
	Args       string `json:"args"`
	Status     string `json:"status"`
	CreatedAt  int64  `json:"createdAt"`
	DecidedAt  int64  `json:"decidedAt,omitempty"`
}

type Routine struct {
	ID        string `json:"id"`
	BotID     string `json:"botId"`
	ThreadID  string `json:"threadId"`
	Name      string `json:"name"`
	Prompt    string `json:"prompt"`
	Cron      string `json:"cron"`
	Enabled   bool   `json:"enabled"`
	NextAt    int64  `json:"nextAt"`
	LastRunAt int64  `json:"lastRunAt"`
	LastError string `json:"lastError,omitempty"`
	WatchPath string `json:"watchPath,omitempty"`
	WatchHash string `json:"-"`
	CreatedAt int64  `json:"createdAt"`
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	// Each constant is one Exec: a single multi-statement Exec stops at the
	// first statement boundary under some drivers, which silently drops tables.
	for _, statement := range []string{schema, memorySchema, loopSchema, auditSchema, boardSchema, shareSchema, attachmentSchema, spaceSchema, groupSchema, dmSchema, mcpServerSchema, secretSchema, msgSchema, pluginSchema, externalAgentSchema, userSchema, walletSchema, appConnSchema} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply schema: %w", err)
		}
	}
	if _, err := db.Exec(`ALTER TABLE bots ADD COLUMN space_id TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		db.Close()
		return nil, fmt.Errorf("migrate bots: %w", err)
	}
	// Backfill: bots that predate spaces join the default space.
	store := &Store{db: db}
	ctx := context.Background()
	space, err := store.DefaultSpace(ctx)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("seed default space: %w", err)
	}
	if _, err := db.Exec(`UPDATE bots SET space_id = ? WHERE space_id = ''`, space.ID); err != nil {
		db.Close()
		return nil, fmt.Errorf("backfill bot spaces: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE instruction_versions ADD COLUMN prev_instructions TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		db.Close()
		return nil, fmt.Errorf("migrate instruction_versions: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE instruction_versions ADD COLUMN seq INTEGER NOT NULL DEFAULT 0`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		db.Close()
		return nil, fmt.Errorf("migrate instruction_versions: %w", err)
	}
	// Backfill the monotonic per-bot sequence for rows written before the seq
	// column existed, using physical insertion order (rowid) within each bot.
	if _, err := db.Exec(`UPDATE instruction_versions SET seq = (
		SELECT COUNT(*) FROM instruction_versions AS older
		WHERE older.bot_id = instruction_versions.bot_id AND older.rowid <= instruction_versions.rowid
	) WHERE seq = 0`); err != nil {
		db.Close()
		return nil, fmt.Errorf("backfill instruction_versions seq: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE routines ADD COLUMN last_error TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		db.Close()
		return nil, fmt.Errorf("migrate routines: %w", err)
	}
	for _, column := range []string{"watch_path", "watch_hash"} {
		if _, err := db.Exec(`ALTER TABLE routines ADD COLUMN ` + column + ` TEXT NOT NULL DEFAULT ''`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			db.Close()
			return nil, fmt.Errorf("migrate routines: %w", err)
		}
	}
	for _, column := range []string{"started_at", "finished_at"} {
		if _, err := db.Exec(`ALTER TABLE runs ADD COLUMN ` + column + ` INTEGER NOT NULL DEFAULT 0`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			db.Close()
			return nil, fmt.Errorf("migrate runs: %w", err)
		}
	}
	for _, column := range []string{"prompt_tokens", "cached_tokens"} {
		if _, err := db.Exec(`ALTER TABLE messages ADD COLUMN ` + column + ` INTEGER NOT NULL DEFAULT 0`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			db.Close()
			return nil, fmt.Errorf("migrate messages usage: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func NewID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return prefix + "_" + fmt.Sprint(time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

func now() int64 { return time.Now().Unix() }

func (s *Store) CreateBot(ctx context.Context, name, instructions, model string) (Bot, error) {
	return s.CreateBotIn(ctx, "", name, instructions, model)
}

// CreateBotIn makes a bot inside one space; empty space means the default.
func (s *Store) CreateBotIn(ctx context.Context, spaceID, name, instructions, model string) (Bot, error) {
	if spaceID == "" {
		space, err := s.DefaultSpace(ctx)
		if err != nil {
			return Bot{}, err
		}
		spaceID = space.ID
	}
	bot := Bot{ID: NewID("bot"), Name: name, Instructions: instructions, Model: model, SpaceID: spaceID, CreatedAt: now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bots (id, name, instructions, model, space_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		bot.ID, bot.Name, bot.Instructions, bot.Model, bot.SpaceID, bot.CreatedAt)
	return bot, err
}

func (s *Store) ListBots(ctx context.Context) ([]Bot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, instructions, model, space_id, created_at FROM bots ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bots := []Bot{}
	for rows.Next() {
		var bot Bot
		if err := rows.Scan(&bot.ID, &bot.Name, &bot.Instructions, &bot.Model, &bot.SpaceID, &bot.CreatedAt); err != nil {
			return nil, err
		}
		bots = append(bots, bot)
	}
	return bots, rows.Err()
}

var ErrNotFound = errors.New("not found")

func (s *Store) GetBot(ctx context.Context, id string) (Bot, error) {
	var bot Bot
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, instructions, model, space_id, created_at FROM bots WHERE id = ?`, id).
		Scan(&bot.ID, &bot.Name, &bot.Instructions, &bot.Model, &bot.SpaceID, &bot.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return bot, ErrNotFound
	}
	return bot, err
}

func (s *Store) UpdateBot(ctx context.Context, id, name, instructions, model string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE bots SET name = ?, instructions = ?, model = ? WHERE id = ?`, name, instructions, model, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateThread(ctx context.Context, botID, title string) (Thread, error) {
	thread := Thread{ID: NewID("thr"), BotID: botID, Title: title, CreatedAt: now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO threads (id, bot_id, title, created_at) VALUES (?, ?, ?, ?)`,
		thread.ID, thread.BotID, thread.Title, thread.CreatedAt)
	return thread, err
}

func (s *Store) ListThreads(ctx context.Context, botID string) ([]Thread, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bot_id, title, created_at FROM threads WHERE bot_id = ? ORDER BY created_at DESC`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	threads := []Thread{}
	for rows.Next() {
		var t Thread
		if err := rows.Scan(&t.ID, &t.BotID, &t.Title, &t.CreatedAt); err != nil {
			return nil, err
		}
		threads = append(threads, t)
	}
	return threads, rows.Err()
}

func (s *Store) GetThread(ctx context.Context, id string) (Thread, error) {
	var t Thread
	err := s.db.QueryRowContext(ctx,
		`SELECT id, bot_id, title, created_at FROM threads WHERE id = ?`, id).
		Scan(&t.ID, &t.BotID, &t.Title, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// FirstThread returns the oldest thread for a bot, creating one when the bot has none.
func (s *Store) FirstThread(ctx context.Context, botID string) (Thread, error) {
	var t Thread
	err := s.db.QueryRowContext(ctx,
		`SELECT id, bot_id, title, created_at FROM threads WHERE bot_id = ? ORDER BY created_at LIMIT 1`, botID).
		Scan(&t.ID, &t.BotID, &t.Title, &t.CreatedAt)
	if err == nil {
		return t, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return t, err
	}
	return s.CreateThread(ctx, botID, "Chat")
}

// DeleteThread removes one thread and its messages. SQLite runs without FK
// enforcement by default, so child rows are deleted explicitly.
func (s *Store) DeleteThread(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE thread_id = ?`, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM threads WHERE id = ?`, id)
	return err
}

func (s *Store) AddMessage(ctx context.Context, msg Message) (Message, error) {
	if msg.ID == "" {
		msg.ID = NewID("msg")
	}
	if msg.CreatedAt == 0 {
		msg.CreatedAt = now()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO messages (id, thread_id, role, content, tool_calls, tool_call_id, tool_name, prompt_tokens, cached_tokens, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.ThreadID, msg.Role, msg.Content, msg.ToolCalls, msg.ToolCallID, msg.ToolName, msg.PromptTokens, msg.CachedTokens, msg.CreatedAt)
	return msg, err
}

func (s *Store) ListMessages(ctx context.Context, threadID string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, thread_id, role, content, tool_calls, tool_call_id, tool_name, prompt_tokens, cached_tokens, created_at
		 FROM messages WHERE thread_id = ? ORDER BY created_at, rowid`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Role, &m.Content, &m.ToolCalls, &m.ToolCallID, &m.ToolName, &m.PromptTokens, &m.CachedTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) CreateRun(ctx context.Context, botID, threadID, trigger, prompt string) (Run, error) {
	run := Run{
		ID: NewID("run"), BotID: botID, ThreadID: threadID, Trigger: trigger,
		Prompt: prompt, Status: RunQueued, CreatedAt: now(), UpdatedAt: now(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runs (id, bot_id, thread_id, trigger, prompt, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.BotID, run.ThreadID, run.Trigger, run.Prompt, run.Status, run.CreatedAt, run.UpdatedAt)
	return run, err
}

const runColumns = `id, bot_id, thread_id, trigger, prompt, status, error, steps, lease_until, created_at, updated_at, started_at, finished_at`

func scanRun(row interface{ Scan(...any) error }) (Run, error) {
	var run Run
	err := row.Scan(&run.ID, &run.BotID, &run.ThreadID, &run.Trigger, &run.Prompt, &run.Status,
		&run.Error, &run.Steps, &run.LeaseUntil, &run.CreatedAt, &run.UpdatedAt, &run.StartedAt, &run.FinishedAt)
	if err == nil && run.StartedAt > 0 && run.StartedAt >= run.CreatedAt {
		run.QueueWaitS = run.StartedAt - run.CreatedAt
	}
	if errors.Is(err, sql.ErrNoRows) {
		return run, ErrNotFound
	}
	return run, err
}

func (s *Store) GetRun(ctx context.Context, id string) (Run, error) {
	return scanRun(s.db.QueryRowContext(ctx, `SELECT `+runColumns+` FROM runs WHERE id = ?`, id))
}

func (s *Store) UpdateRun(ctx context.Context, id, status, errText string, steps int) error {
	stamp := now()
	if status == RunRunning {
		_, err := s.db.ExecContext(ctx,
			`UPDATE runs SET status = ?, error = ?, steps = ?, updated_at = ?, started_at = COALESCE(NULLIF(started_at, 0), ?) WHERE id = ?`,
			status, errText, steps, stamp, stamp, id)
		return err
	}
	if status == RunDone || status == RunFailed || status == RunCancelled {
		_, err := s.db.ExecContext(ctx,
			`UPDATE runs SET status = ?, error = ?, steps = ?, updated_at = ?, finished_at = ? WHERE id = ?`,
			status, errText, steps, stamp, stamp, id)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE runs SET status = ?, error = ?, steps = ?, updated_at = ? WHERE id = ?`,
		status, errText, steps, stamp, id)
	return err
}

// ClaimRun takes a lease so two processes never run the same turn at once.
func (s *Store) ClaimRun(ctx context.Context, id string, lease time.Duration) (Run, error) {
	until := time.Now().Add(lease).Unix()
	res, err := s.db.ExecContext(ctx,
		`UPDATE runs SET lease_until = ?, updated_at = ? WHERE id = ? AND lease_until < ?`,
		until, now(), id, time.Now().Unix())
	if err != nil {
		return Run{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Run{}, errors.New("run is leased by another worker")
	}
	return s.GetRun(ctx, id)
}

// ReleaseRun drops the lease so a paused run can resume immediately.
func (s *Store) ReleaseRun(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET lease_until = 0 WHERE id = ?`, id)
	return err
}

func (s *Store) ListRuns(ctx context.Context, threadID string, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+runColumns+` FROM runs WHERE thread_id = ? ORDER BY created_at DESC LIMIT ?`, threadID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (s *Store) CreateApproval(ctx context.Context, runID, threadID, toolCallID, tool, args string) (Approval, error) {
	a := Approval{
		ID: NewID("apv"), RunID: runID, ThreadID: threadID, ToolCallID: toolCallID,
		Tool: tool, Args: args, Status: ApprovalPending, CreatedAt: now(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO approvals (id, run_id, thread_id, tool_call_id, tool, args, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(run_id, tool_call_id) DO NOTHING`,
		a.ID, a.RunID, a.ThreadID, a.ToolCallID, a.Tool, a.Args, a.Status, a.CreatedAt)
	if err != nil {
		return a, err
	}
	return a, nil
}

const approvalColumns = `id, run_id, thread_id, tool_call_id, tool, args, status, created_at, decided_at`

func scanApproval(row interface{ Scan(...any) error }) (Approval, error) {
	var a Approval
	err := row.Scan(&a.ID, &a.RunID, &a.ThreadID, &a.ToolCallID, &a.Tool, &a.Args, &a.Status, &a.CreatedAt, &a.DecidedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func (s *Store) GetApproval(ctx context.Context, id string) (Approval, error) {
	return scanApproval(s.db.QueryRowContext(ctx, `SELECT `+approvalColumns+` FROM approvals WHERE id = ?`, id))
}

func (s *Store) ApprovalForCall(ctx context.Context, runID, toolCallID string) (Approval, error) {
	return scanApproval(s.db.QueryRowContext(ctx,
		`SELECT `+approvalColumns+` FROM approvals WHERE run_id = ? AND tool_call_id = ?`, runID, toolCallID))
}

func (s *Store) DecideApproval(ctx context.Context, id, decision string) (Approval, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE approvals SET status = ?, decided_at = ? WHERE id = ? AND status = ?`,
		decision, now(), id, ApprovalPending)
	if err != nil {
		return Approval{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Approval{}, errors.New("approval already decided")
	}
	return s.GetApproval(ctx, id)
}

func (s *Store) ListApprovals(ctx context.Context, threadID string) ([]Approval, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+approvalColumns+` FROM approvals WHERE thread_id = ? ORDER BY created_at DESC LIMIT 50`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Approval{}
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) CreateRoutine(ctx context.Context, botID, threadID, name, prompt, cronExpr string, nextAt int64) (Routine, error) {
	r := Routine{
		ID: NewID("rtn"), BotID: botID, ThreadID: threadID, Name: name, Prompt: prompt,
		Cron: cronExpr, Enabled: true, NextAt: nextAt, CreatedAt: now(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO routines (id, bot_id, thread_id, name, prompt, cron, enabled, next_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.BotID, r.ThreadID, r.Name, r.Prompt, r.Cron, boolInt(r.Enabled), r.NextAt, r.CreatedAt)
	return r, err
}

func (s *Store) ListRoutines(ctx context.Context, botID string) ([]Routine, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bot_id, thread_id, name, prompt, cron, enabled, next_at, last_run_at, last_error, watch_path, watch_hash, created_at
		 FROM routines WHERE bot_id = ? ORDER BY created_at`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Routine{}
	for rows.Next() {
		var r Routine
		var enabled int
		if err := rows.Scan(&r.ID, &r.BotID, &r.ThreadID, &r.Name, &r.Prompt, &r.Cron, &enabled, &r.NextAt, &r.LastRunAt, &r.LastError, &r.WatchPath, &r.WatchHash, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) DeleteRoutine(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM routines WHERE id = ?`, id)
	return err
}

// RoutineForThread finds the routine whose schedule owns a thread+prompt pair,
// so a failed scheduled run can annotate its routine row.
func (s *Store) RoutineForThread(ctx context.Context, threadID, prompt string) (*Routine, error) {
	var r Routine
	var enabled int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, bot_id, thread_id, name, prompt, cron, enabled, next_at, last_run_at, last_error, watch_path, watch_hash, created_at
		 FROM routines WHERE thread_id = ? AND prompt = ? ORDER BY created_at DESC LIMIT 1`, threadID, prompt).
		Scan(&r.ID, &r.BotID, &r.ThreadID, &r.Name, &r.Prompt, &r.Cron, &enabled, &r.NextAt, &r.LastRunAt, &r.LastError, &r.WatchPath, &r.WatchHash, &r.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	r.Enabled = enabled != 0
	return &r, nil
}

// ListRoutineRuns returns the recent runs for a routine's thread, newest
// first: the routine's run history.
func (s *Store) ListRoutineRuns(ctx context.Context, routineID string, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 10
	}
	routine, err := s.GetRoutine(ctx, routineID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+runColumns+` FROM runs WHERE thread_id = ? AND trigger = 'routine' ORDER BY created_at DESC LIMIT ?`, routine.ThreadID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// SearchMessages finds message content across every bot, newest first,
// capped at limit. Used by space search: one query, all conversations.
func (s *Store) SearchMessages(ctx context.Context, query string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, thread_id, role, content, tool_calls, tool_call_id, tool_name, created_at FROM messages
		 WHERE content LIKE '%' || ? || '%' ORDER BY created_at DESC LIMIT ?`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Role, &m.Content, &m.ToolCalls, &m.ToolCallID, &m.ToolName, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetRoutine returns one routine by id.
func (s *Store) GetRoutine(ctx context.Context, id string) (Routine, error) {
	var r Routine
	var enabled int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, bot_id, thread_id, name, prompt, cron, enabled, next_at, last_run_at, last_error, watch_path, watch_hash, created_at
		 FROM routines WHERE id = ?`, id).
		Scan(&r.ID, &r.BotID, &r.ThreadID, &r.Name, &r.Prompt, &r.Cron, &enabled, &r.NextAt, &r.LastRunAt, &r.LastError, &r.WatchPath, &r.WatchHash, &r.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return Routine{}, ErrNotFound
		}
		return Routine{}, err
	}
	r.Enabled = enabled != 0
	return r, nil
}

// DueRoutines returns routines whose next tick has passed.
func (s *Store) DueRoutines(ctx context.Context, at int64) ([]Routine, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bot_id, thread_id, name, prompt, cron, enabled, next_at, last_run_at, last_error, watch_path, watch_hash, created_at
		 FROM routines WHERE enabled = 1 AND next_at > 0 AND next_at <= ? ORDER BY next_at LIMIT 20`, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Routine{}
	for rows.Next() {
		var r Routine
		var enabled int
		if err := rows.Scan(&r.ID, &r.BotID, &r.ThreadID, &r.Name, &r.Prompt, &r.Cron, &enabled, &r.NextAt, &r.LastRunAt, &r.LastError, &r.WatchPath, &r.WatchHash, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) MarkRoutineFired(ctx context.Context, id string, firedAt, nextAt int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE routines SET last_run_at = ?, next_at = ?, last_error = '' WHERE id = ?`, firedAt, nextAt, id)
	return err
}

// RecordRoutineError keeps the last failure on the routine row so the UI can
// show why the most recent scheduled run failed.
func (s *Store) RecordRoutineError(ctx context.Context, id, message string) error {
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE routines SET last_error = ? WHERE id = ?`, message, id)
	return err
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return value, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// tokens splits a query into lowercase search words, dropping punctuation so a
// user can type "smtp port" and match "SMTP:587 (port)".
func tokens(query string) []string {
	var out []string
	for _, raw := range strings.Fields(strings.ToLower(query)) {
		word := strings.Trim(raw, ".,:;!?\"'()[]{}<>/\\-_")
		if word != "" {
			out = append(out, word)
		}
	}
	return out
}
