package httpapi

import (
	"net/http"
	"strings"
	"time"

	"gobot/internal/schedule"
)

// updateRoutine edits a routine: name, prompt, cron and enabled. A cron
// change recomputes the next firing time.
func (s *Server) updateRoutine(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    *string `json:"name"`
		Prompt  *string `json:"prompt"`
		Cron    *string `json:"cron"`
		Enabled *bool   `json:"enabled"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	var nextAt *int64
	if body.Cron != nil {
		parsed, err := schedule.Parse(strings.TrimSpace(*body.Cron))
		if err != nil {
			writeError(w, http.StatusBadRequest, "cron: "+err.Error())
			return
		}
		next, err := parsed.Next(time.Now())
		if err != nil {
			writeError(w, http.StatusBadRequest, "cron: "+err.Error())
			return
		}
		unix := next.Unix()
		nextAt = &unix
	}
	routine, err := s.Store.UpdateRoutine(r.Context(), r.PathValue("id"), body.Name, body.Prompt, body.Cron, body.Enabled, nextAt)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "routine.update", r.PathValue("id"), "")
	writeJSON(w, http.StatusOK, routine)
}
