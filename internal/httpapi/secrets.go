package httpapi

import (
	"net/http"
	"strings"
)

// listSecrets returns secret names only. Values never leave the vault except
// inside a use_secret tool call.
func (s *Server) listSecrets(w http.ResponseWriter, r *http.Request) {
	secrets, err := s.Store.ListSecrets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, secrets)
}

// setSecret stores one named secret, encrypted at rest.
func (s *Server) setSecret(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Name) == "" || body.Value == "" {
		writeError(w, http.StatusBadRequest, "name and value are required")
		return
	}
	if err := s.Store.SetSecret(r.Context(), strings.TrimSpace(body.Name), body.Value); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "secret.set", strings.TrimSpace(body.Name), "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "stored"})
}

// deleteSecret removes one named secret.
func (s *Server) deleteSecret(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteSecret(r.Context(), r.PathValue("name")); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "secret.delete", r.PathValue("name"), "")
	w.WriteHeader(http.StatusNoContent)
}
