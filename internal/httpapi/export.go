package httpapi

import (
	"encoding/json"
	"net/http"
)

// exportAll downloads the whole install as one JSON archive.
func (s *Server) exportAll(w http.ResponseWriter, r *http.Request) {
	archive, err := s.Store.ExportAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "backup.export", "", "")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=gobot-backup.json")
	_ = json.NewEncoder(w).Encode(archive)
}

// importAll restores an archive into this store. IDs are preserved.
func (s *Server) importAll(w http.ResponseWriter, r *http.Request) {
	var raw json.RawMessage
	if !readJSON(w, r, &raw) {
		return
	}
	if err := s.Store.ImportAll(r.Context(), raw); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "backup.import", "", "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
