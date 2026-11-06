package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gobot/internal/tools"
)

// listPlugins returns one bot's plugins.
func (s *Server) listPlugins(w http.ResponseWriter, r *http.Request) {
	plugins, err := s.Store.ListPlugins(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plugins)
}

// installPlugin records one folder for one bot after validating the manifest.
func (s *Server) installPlugin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	path := strings.TrimSpace(body.Path)
	if path == "" || strings.Contains(path, "..") {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	abs := filepath.Join(s.Engine.PluginsDir(), filepath.Clean("/"+path))
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "folder not found under plugins/")
		return
	}
	manifest, err := tools.LoadManifest(abs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	plugin, err := s.Store.InstallPlugin(r.Context(), manifest.Name, path, r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "plugin.install", plugin.ID, path)
	writeJSON(w, http.StatusCreated, plugin)
}

// setPluginEnabled flips one plugin.
func (s *Server) setPluginEnabled(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.Store.SetPluginEnabled(r.Context(), r.PathValue("id"), body.Enabled); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "plugin.toggle", r.PathValue("id"), "")
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": body.Enabled})
}

// deletePlugin uninstalls one plugin.
func (s *Server) deletePlugin(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeletePlugin(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "plugin.delete", r.PathValue("id"), "")
	w.WriteHeader(http.StatusNoContent)
}
