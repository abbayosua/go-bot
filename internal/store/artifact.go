package store

import (
	"context"
	"strings"
)

// Artifact is a file the model produced, surfaced as an openable row:
// workspace-relative path plus size. The feed resolves content from disk,
// so the database never carries file bytes.
type Artifact struct {
	ThreadID string `json:"threadId"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	ToolCall string `json:"toolCallId,omitempty"`
}

// ListArtifacts returns files written by write_file tool calls in a thread,
// newest first. Paths stay workspace-relative; callers resolve them.
func (s *Store) ListArtifacts(ctx context.Context, threadID string) ([]Artifact, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tool_call_id, content FROM messages
		 WHERE thread_id = ? AND role = 'tool' AND tool_name = 'write_file'
		 ORDER BY created_at DESC, rowid DESC LIMIT 50`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Artifact{}
	for rows.Next() {
		var callID, content string
		if err := rows.Scan(&callID, &content); err != nil {
			return nil, err
		}
		path, size := parseArtifact(content)
		if path == "" {
			continue
		}
		out = append(out, Artifact{ThreadID: threadID, Path: path, Size: size, ToolCall: callID})
	}
	return out, rows.Err()
}

// parseArtifact reads the write_file result shape: "wrote <n> bytes to <path>".
func parseArtifact(content string) (string, int64) {
	text := strings.TrimSpace(content)
	rest, ok := strings.CutPrefix(text, "wrote ")
	if !ok {
		return "", 0
	}
	middle, path, ok := strings.Cut(rest, " bytes to ")
	if !ok {
		return "", 0
	}
	var size int64 = -1
	for _, field := range strings.Fields(middle) {
		var n int64
		valid := true
		for _, digit := range field {
			if digit < '0' || digit > '9' {
				valid = false
				break
			}
			n = n*10 + int64(digit-'0')
		}
		if valid {
			size = n
		}
	}
	if size < 0 {
		return "", 0
	}
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "..") {
		return "", 0
	}
	return path, size
}
