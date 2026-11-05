package tools

import "path/filepath"

// Resolve is the exported workspace path resolver: workspace-relative path
// to absolute, never escaping the bot's workspace (symlinks included).
func Resolve(workspaces, botID, rel string) (string, error) {
	return resolve(filepath.Join(workspaces, botID), rel)
}
