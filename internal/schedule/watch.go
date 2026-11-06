package schedule

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Fingerprint returns a stable hash of a file or directory tree: paths,
// sizes and mtimes for dirs, content hash for a single file. Missing paths
// hash as the empty marker so a created file counts as a change.
// Fingerprint returns a stable hash of a file or directory tree. The first
// argument is the bot workspace root; botID is kept for the legacy
// (workspaces, botID, rel) call shape and ignored when empty.
func Fingerprint(workspaces, botID, rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "/" || strings.Contains(rel, "..") {
		return "refused"
	}
	clean := filepath.Clean("/" + rel)
	if clean == "/" {
		return "refused"
	}
	abs := filepath.Join(workspaces, filepath.FromSlash(clean))
	if botID != "" {
		abs = filepath.Join(workspaces, botID, filepath.FromSlash(clean))
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "missing"
	}
	h := sha256.New()
	if !info.IsDir() {
		h.Write([]byte("file:"))
		content, err := os.ReadFile(abs)
		if err != nil {
			return fmt.Sprintf("unreadable:%d", info.ModTime().UnixNano())
		}
		h.Write(content)
		return fmt.Sprintf("%x", h.Sum(nil))
	}
	var entries []string
	_ = filepath.WalkDir(abs, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return nil
		}
		stat, err := entry.Info()
		if err != nil {
			return nil
		}
		entries = append(entries, fmt.Sprintf("%s:%d:%d", rel, stat.Size(), stat.ModTime().UnixNano()))
		return nil
	})
	sort.Strings(entries)
	for _, entry := range entries {
		h.Write([]byte("entry:"))
		h.Write([]byte(entry))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
