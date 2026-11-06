package httpapi

import (
	"net/http"
	"strings"
)

// getFleetSwitch reports whether a bot's fleet tools are on.
func (s *Server) getFleetSwitch(w http.ResponseWriter, r *http.Request) {
	value, _ := s.Store.GetSetting(r.Context(), "fleet:"+r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"fleet": value == "on"})
}

// setFleetSwitch flips one bot's fleet tools. Only "on" enables.
func (s *Server) setFleetSwitch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Fleet bool `json:"fleet"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	value := "off"
	if body.Fleet {
		value = "on"
	}
	if err := s.Store.SetSetting(r.Context(), "fleet:"+r.PathValue("id"), value); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "fleet.switch", r.PathValue("id"), strings.TrimSpace(value))
	writeJSON(w, http.StatusOK, map[string]bool{"fleet": body.Fleet})
}
