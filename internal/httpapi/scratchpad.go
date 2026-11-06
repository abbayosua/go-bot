package httpapi

import (
	"net/http"
	"strings"
)

// scratchKey scopes working notes per bot in the settings table.
func scratchKey(botID string) string { return "scratch:" + botID }

// getScratchpad returns one bot's working notes as line items.
func (s *Server) getScratchpad(w http.ResponseWriter, r *http.Request) {
	notes, err := s.Store.GetSetting(r.Context(), scratchKey(r.PathValue("id")))
	if err != nil || strings.TrimSpace(notes) == "" {
		writeJSON(w, http.StatusOK, []string{})
		return
	}
	lines := []string{}
	for _, line := range strings.Split(notes, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	writeJSON(w, http.StatusOK, lines)
}

// addScratchpad appends one note line.
func (s *Server) addScratchpad(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	key := scratchKey(r.PathValue("id"))
	existing, _ := s.Store.GetSetting(r.Context(), key)
	combined := existing
	if strings.TrimSpace(combined) != "" {
		combined += "\n"
	}
	combined += "- " + strings.TrimSpace(body.Text)
	if len(combined) > 8000 {
		combined = combined[len(combined)-8000:]
	}
	if err := s.Store.SetSetting(r.Context(), key, combined); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "noted"})
}

// clearScratchpad wipes one bot's working notes in one action.
func (s *Server) clearScratchpad(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.SetSetting(r.Context(), scratchKey(r.PathValue("id")), ""); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}
