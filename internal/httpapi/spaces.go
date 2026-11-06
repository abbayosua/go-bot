package httpapi

import (
	"net/http"
	"strings"
)

// listSpaces returns every space, oldest first.
func (s *Server) listSpaces(w http.ResponseWriter, r *http.Request) {
	spaces, err := s.Store.ListSpaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, spaces)
}

// createSpace makes one named space.
func (s *Server) createSpace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	space, err := s.Store.CreateSpace(r.Context(), strings.TrimSpace(body.Name))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "space.create", space.ID, space.Name)
	writeJSON(w, http.StatusCreated, space)
}

// deleteSpace removes an empty space; spaces holding bots are refused.
func (s *Server) deleteSpace(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteSpace(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "space.delete", r.PathValue("id"), "")
	w.WriteHeader(http.StatusNoContent)
}
