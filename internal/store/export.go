package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

// Export is one archive with everything needed to move an install: bots,
// threads, messages, runs, memory, knowledge, skills, routines and settings.
// Runs and audit rows larger than history are excluded by design; approvals
// travel with their threads because they gate resume.
type Export struct {
	Version  int               `json:"version"`
	Bots     []Bot             `json:"bots"`
	Threads  []Thread          `json:"threads"`
	Messages []Message         `json:"messages"`
	Routines []Routine         `json:"routines"`
	Memories []Memory          `json:"memories"`
	Pages    []Knowledge       `json:"knowledge"`
	Skills   []Skill           `json:"skills"`
	Settings map[string]string `json:"settings"`
}

const exportVersion = 1

// ExportAll reads the whole install into one archive.
func (s *Store) ExportAll(ctx context.Context) (Export, error) {
	out := Export{Version: exportVersion, Settings: map[string]string{}}
	var err error
	if out.Bots, err = s.ListBots(ctx); err != nil {
		return out, err
	}
	for _, bot := range out.Bots {
		var threads []Thread
		if threads, err = s.ListThreads(ctx, bot.ID); err != nil {
			return out, err
		}
		for _, thread := range threads {
			out.Threads = append(out.Threads, thread)
			var messages []Message
			if messages, err = s.ListMessages(ctx, thread.ID); err != nil {
				return out, err
			}
			out.Messages = append(out.Messages, messages...)
		}
		var routines []Routine
		if routines, err = s.ListRoutines(ctx, bot.ID); err != nil {
			return out, err
		}
		out.Routines = append(out.Routines, routines...)
		var memories []Memory
		if memories, err = s.ListMemories(ctx, bot.ID, 1000); err != nil {
			return out, err
		}
		out.Memories = append(out.Memories, memories...)
		var pages []Knowledge
		if pages, err = s.ListKnowledge(ctx, bot.ID); err != nil {
			return out, err
		}
		out.Pages = append(out.Pages, pages...)
	}
	if out.Skills, err = s.ListSkills(ctx); err != nil {
		return out, err
	}
	for _, key := range []string{"approval_mode", "provider_base_url", "provider_model"} {
		if value, serr := s.GetSetting(ctx, key); serr == nil && value != "" {
			out.Settings[key] = value
		}
	}
	return out, nil
}

// ImportAll restores an archive into an empty store. IDs are preserved so
// cross-table references survive the round trip.
func (s *Store) ImportAll(ctx context.Context, raw json.RawMessage) error {
	var in Export
	if err := json.Unmarshal(raw, &in); err != nil {
		return err
	}
	if in.Version != exportVersion {
		return ErrNotFound
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		for _, bot := range in.Bots {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO bots (id, name, instructions, model, created_at) VALUES (?, ?, ?, ?, ?)`,
				bot.ID, bot.Name, bot.Instructions, bot.Model, bot.CreatedAt); err != nil {
				return err
			}
		}
		for _, thread := range in.Threads {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO threads (id, bot_id, title, created_at) VALUES (?, ?, ?, ?)`,
				thread.ID, thread.BotID, thread.Title, thread.CreatedAt); err != nil {
				return err
			}
		}
		for _, msg := range in.Messages {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO messages (id, thread_id, role, content, tool_calls, tool_call_id, tool_name, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				msg.ID, msg.ThreadID, msg.Role, msg.Content, msg.ToolCalls, msg.ToolCallID, msg.ToolName, msg.CreatedAt); err != nil {
				return err
			}
		}
		for _, routine := range in.Routines {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO routines (id, bot_id, thread_id, name, prompt, cron, enabled, next_at, last_run_at, last_error, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				routine.ID, routine.BotID, routine.ThreadID, routine.Name, routine.Prompt, routine.Cron, boolInt(routine.Enabled), routine.NextAt, routine.LastRunAt, routine.LastError, routine.CreatedAt); err != nil {
				return err
			}
		}
		for _, mem := range in.Memories {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO memories (id, bot_id, content, created_at) VALUES (?, ?, ?, ?)`,
				mem.ID, mem.BotID, mem.Content, mem.CreatedAt); err != nil {
				return err
			}
		}
		for _, page := range in.Pages {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO knowledge (id, bot_id, title, body, created_at) VALUES (?, ?, ?, ?, ?)`,
				page.ID, page.BotID, page.Title, page.Body, page.CreatedAt); err != nil {
				return err
			}
		}
		for _, skill := range in.Skills {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO skills (id, name, description, body, created_at) VALUES (?, ?, ?, ?, ?)`,
				skill.ID, skill.Name, skill.Description, skill.Body, skill.CreatedAt); err != nil {
				return err
			}
		}
		for key, value := range in.Settings {
			if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)`, key, value); err != nil {
				return err
			}
		}
		return nil
	})
}
