package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// importAgent turns another assistant's folder into a bot: instructions.md
// becomes the system instructions, memory.md lines become memories, and each
// skills/*.md file becomes a shared skill. Paths stay inside the upload root.
func (s *Server) importAgent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	root := s.Engine.WorkspacesRoot()
	clean := filepath.Clean("/" + strings.TrimSpace(body.Path))
	if clean == "/" || strings.Contains(clean, "..") {
		writeError(w, http.StatusBadRequest, "path escapes the workspace")
		return
	}
	// Imports live under an imports/ directory at the data root.
	abs := filepath.Join(filepath.Dir(root), "imports", filepath.FromSlash(clean))
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "folder not found under imports/")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = filepath.Base(clean)
	}
	instructions := ""
	if raw, err := os.ReadFile(filepath.Join(abs, "instructions.md")); err == nil {
		instructions = string(raw)
		if len(instructions) > 8000 {
			instructions = instructions[:8000]
		}
	}
	bot, err := s.Store.CreateBot(r.Context(), name, instructions, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	imported := map[string]int{}
	if raw, err := os.ReadFile(filepath.Join(abs, "memory.md")); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				if _, err := s.Store.AddMemory(r.Context(), bot.ID, trimmed); err == nil {
					imported["memories"]++
				}
			}
		}
	}
	entries, _ := os.ReadDir(filepath.Join(abs, "skills"))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(abs, "skills", entry.Name()))
		if err != nil {
			continue
		}
		title := strings.TrimSuffix(entry.Name(), ".md")
		if _, err := s.Store.CreateSkill(r.Context(), title, "Imported skill.", string(raw)); err == nil {
			imported["skills"]++
		}
	}
	if _, err := s.Store.CreateThread(r.Context(), bot.ID, "Chat"); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "agent.import", bot.ID, body.Path)
	writeJSON(w, http.StatusCreated, map[string]any{"bot": bot, "imported": imported})
}
