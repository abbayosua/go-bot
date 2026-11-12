package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gobot/internal/tools"
)

// listArtifacts returns the files the model produced in a thread.
func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request) {
	artifacts, err := s.Store.ListArtifacts(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, artifacts)
}

// downloadArtifact serves one artifact file from the bot's workspace.
func (s *Server) downloadArtifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	thread, err := s.Store.GetThread(r.Context(), id)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	abs, err := tools.Resolve(s.Engine.WorkspacesRoot(), thread.BotID, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(abs))
	http.ServeFile(w, r, abs)
}

// previewArtifact serves one artifact file inline for the in-chat preview
// pane. Unlike downloadArtifact it never forces a download: the browser
// renders the file inside a sandboxed iframe, so the response carries a
// sandboxing CSP, no-sniff and no-store.
func (s *Server) previewArtifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	thread, err := s.Store.GetThread(r.Context(), id)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	abs, err := tools.Resolve(s.Engine.WorkspacesRoot(), thread.BotID, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	w.Header().Set("Content-Security-Policy", "sandbox allow-scripts; default-src 'none'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; media-src 'self' data: blob:; connect-src 'self'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, abs)
}
