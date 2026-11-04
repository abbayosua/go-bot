package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gobot/internal/llm"
)

const shellTimeout = 120 * time.Second

// RefusedCommand is returned to the model as a tool error it can read and route around.
type RefusedCommand struct{ Reason string }

func (e *RefusedCommand) Error() string { return "command refused: " + e.Reason }

var dangerousPatterns = []struct {
	needle string
	reason string
}{
	{"rm -rf /", "would delete the filesystem root"},
	{"rm -fr /", "would delete the filesystem root"},
	{"mkfs", "would format a device"},
	{"dd of=/dev/", "would write directly to a block device"},
	{"> /dev/sd", "would write directly to a block device"},
	{":(){", "fork bomb"},
	{"shutdown", "would power off the host"},
	{"reboot", "would restart the host"},
	{"chmod -R 777 /", "would change permissions across the host"},
	{"sudo ", "runs as root"},
}

func inspectCommand(command string) error {
	normalized := strings.Join(strings.Fields(command), " ")
	for _, pattern := range dangerousPatterns {
		if strings.Contains(normalized, pattern.needle) {
			return &RefusedCommand{Reason: pattern.reason + "; run something narrower"}
		}
	}
	return nil
}

type shellTool struct{ root string }

func (t *shellTool) Name() string { return "shell" }

func (t *shellTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "shell",
		Description: "Run a shell command inside the bot's workspace and return its combined output.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {"type": "string", "description": "Command line to run with bash -lc."}
			},
			"required": ["command"]
		}`),
	}}
}

func (t *shellTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Command) == "" {
		return "", errors.New("command is required")
	}
	if err := inspectCommand(input.Command); err != nil {
		return "", err
	}
	runCtx, cancel := context.WithTimeout(ctx, shellTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "bash", "-lc", input.Command)
	cmd.Dir = t.root
	cmd.Env = append(os.Environ(),
		"HOME="+t.root,
		"PWD="+t.root,
		"GOBOT_WORKSPACE="+t.root,
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if runCtx.Err() == context.DeadlineExceeded {
		return out + fmt.Sprintf("\n[command timed out after %s]", shellTimeout), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Sprintf("%s\n[exit status %d]", out, exitErr.ExitCode()), nil
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) == "" {
		return "[no output]", nil
	}
	return out, nil
}

type readFileTool struct{ root string }

func (t *readFileTool) Name() string { return "read_file" }

func (t *readFileTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "read_file",
		Description: "Read a UTF-8 text file from the workspace.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "Workspace-relative path."}
			},
			"required": ["path"]
		}`),
	}}
}

func (t *readFileTool) Run(_ context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	target, err := resolve(t.root, input.Path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("path is a directory; use list_files")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type writeFileTool struct{ root string }

func (t *writeFileTool) Name() string { return "write_file" }

func (t *writeFileTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "write_file",
		Description: "Write a text file in the workspace, replacing any existing file.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "Workspace-relative path."},
				"content": {"type": "string", "description": "Full file contents."}
			},
			"required": ["path", "content"]
		}`),
	}}
}

func (t *writeFileTool) Run(_ context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Path) == "" {
		return "", errors.New("path is required")
	}
	target, err := resolve(t.root, input.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, []byte(input.Content), 0o644); err != nil {
		return "", err
	}
	if err := ensureInside(t.root, target); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(input.Content), input.Path), nil
}

func ensureInside(root, target string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return err
	}
	if realTarget != realRoot && !strings.HasPrefix(realTarget, realRoot+string(filepath.Separator)) {
		return errors.New("path escapes the workspace")
	}
	return nil
}

type listFilesTool struct{ root string }

func (t *listFilesTool) Name() string { return "list_files" }

func (t *listFilesTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "list_files",
		Description: "List files and directories under a workspace path.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "Workspace-relative directory; defaults to the workspace root."}
			}
		}`),
	}}
}

func (t *listFilesTool) Run(_ context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return "", err
		}
	}
	target, err := resolve(t.root, input.Path)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		marker := ""
		if entry.IsDir() {
			marker = "/"
		}
		size := ""
		if info, statErr := entry.Info(); statErr == nil && !entry.IsDir() {
			size = fmt.Sprintf(" (%d bytes)", info.Size())
		}
		lines = append(lines, entry.Name()+marker+size)
	}
	if len(lines) == 0 {
		return "[empty directory]", nil
	}
	return strings.Join(lines, "\n"), nil
}
