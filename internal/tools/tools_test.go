package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func registry(t *testing.T) (*Registry, string) {
	t.Helper()
	root := t.TempDir()
	return NewRegistry(root), root
}

func TestWriteReadRoundTrip(t *testing.T) {
	reg, root := registry(t)
	ctx := context.Background()

	if _, err := reg.Run(ctx, "write_file", json.RawMessage(`{"path":"notes/a.txt","content":"hello"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := filepath.Join(root, "notes", "a.txt"); !exists(got) {
		t.Fatalf("%s was not created", got)
	}
	out, err := reg.Run(ctx, "read_file", json.RawMessage(`{"path":"notes/a.txt"}`))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out != "hello" {
		t.Errorf("read returned %q", out)
	}
}

func TestPathEscapesAreRefused(t *testing.T) {
	reg, _ := registry(t)
	ctx := context.Background()
	for _, path := range []string{"../outside.txt", "/etc/passwd", "a/../../b"} {
		raw, _ := json.Marshal(map[string]string{"path": path, "content": "x"})
		if _, err := reg.Run(ctx, "write_file", raw); err == nil {
			t.Errorf("write to %q succeeded, want an error", path)
		}
	}
}

func TestSymlinkEscapeIsRefused(t *testing.T) {
	reg, root := registry(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	raw, _ := json.Marshal(map[string]string{"path": "link/escape.txt", "content": "x"})
	if _, err := reg.Run(context.Background(), "write_file", raw); err == nil {
		t.Error("write through an escaping symlink succeeded")
	}
}

func TestDangerousCommandsAreRefused(t *testing.T) {
	reg, _ := registry(t)
	for _, command := range []string{"rm -rf /", "sudo rm x", "mkfs.ext4 /dev/sda", "shutdown -h now"} {
		raw, _ := json.Marshal(map[string]string{"command": command})
		_, err := reg.Run(context.Background(), "shell", raw)
		var refused *RefusedCommand
		if !errors.As(err, &refused) {
			t.Errorf("%q was not refused: %v", command, err)
		}
	}
}

func TestShellRunsInWorkspace(t *testing.T) {
	reg, root := registry(t)
	out, err := reg.Run(context.Background(), "shell", json.RawMessage(`{"command":"pwd"}`))
	if err != nil {
		t.Fatalf("shell: %v", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	reported := strings.TrimSpace(out)
	if resolvedReported, err := filepath.EvalSymlinks(reported); err == nil {
		reported = resolvedReported
	}
	if reported != resolvedRoot {
		t.Errorf("pwd returned %q, want %q", reported, resolvedRoot)
	}
}

func TestListFiles(t *testing.T) {
	reg, _ := registry(t)
	ctx := context.Background()
	if _, err := reg.Run(ctx, "write_file", json.RawMessage(`{"path":"b.txt","content":"x"}`)); err != nil {
		t.Fatal(err)
	}
	out, err := reg.Run(ctx, "list_files", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "b.txt") {
		t.Errorf("listing %q does not include b.txt", out)
	}
}

func TestSpecsAreSortedAndComplete(t *testing.T) {
	reg, _ := registry(t)
	specs := reg.Specs()
	if len(specs) != 5 {
		t.Fatalf("got %d specs, want 5", len(specs))
	}
	for i := 1; i < len(specs); i++ {
		if specs[i-1].Function.Name > specs[i].Function.Name {
			t.Error("specs are not sorted")
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
