package httpapi

import (
	"net/http"
	"strings"

	"gobot/internal/schedule"
)

// setRoutineWatch attaches a workspace-relative watch path to a routine,
// recording the current fingerprint as the no-change baseline.
func (s *Server) setRoutineWatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	path := strings.TrimSpace(body.Path)
	routine, err := s.Store.GetRoutine(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	hash := ""
	if path != "" {
		hash = schedule.Fingerprint(s.Engine.WorkspacesRoot(), routine.BotID, path)
		if hash == "refused" {
			writeError(w, http.StatusBadRequest, "path escapes the workspace")
			return
		}
	}
	if err := s.Store.SetRoutineWatch(r.Context(), routine.ID, path, hash); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated, err := s.Store.GetRoutine(r.Context(), routine.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "routine.watch", r.PathValue("id"), path)
	writeJSON(w, http.StatusOK, updated)
}
