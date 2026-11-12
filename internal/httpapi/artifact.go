package httpapi

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// resolveArtifact maps a thread plus a workspace-relative path to an absolute
// file, mirroring the engine's modern spaces/<space>/bots/<bot> layout with a
// fallback to the legacy spaces/<bot> layout.
func (s *Server) resolveArtifact(ctx context.Context, threadID, rel string) (string, error) {
	thread, err := s.Store.GetThread(ctx, threadID)
	if err != nil {
		return "", err
	}
	bot, err := s.Store.GetBot(ctx, thread.BotID)
	if err != nil {
		return "", err
	}
	root := s.Engine.WorkspacesRoot()
	abs, err := tools.Resolve(filepath.Join(root, "spaces", bot.SpaceID, "bots"), bot.ID, rel)
	if err == nil {
		if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() {
			return abs, nil
		}
	}
	return tools.Resolve(root, bot.ID, rel)
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
	abs, err := s.resolveArtifact(r.Context(), thread.ID, rel)
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
	if _, err := s.Store.GetThread(r.Context(), id); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	abs, err := s.resolveArtifact(r.Context(), id, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	w.Header().Set("Content-Security-Policy", "sandbox allow-scripts; default-src 'self' https: data: blob:; style-src 'self' 'unsafe-inline' https:; script-src 'self' 'unsafe-inline' https:; img-src 'self' https: data: blob:; font-src 'self' https: data:; media-src 'self' https: data: blob:; connect-src 'self' https:")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasSuffix(strings.ToLower(abs), ".html") || strings.HasSuffix(strings.ToLower(abs), ".htm") {
		serveHTMLWithBase(w, r, abs, "/api/threads/"+id+"/preview?path=")
		return
	}
	http.ServeFile(w, r, abs)
}

// serveHTMLWithBase serves an HTML artifact with a <base> tag injected so
// that relative asset links (style.css, app.js) resolve against the thread's
// preview endpoint instead of the bare /api/threads/<id>/ path.
func serveHTMLWithBase(w http.ResponseWriter, r *http.Request, abs, prefix string) {
	raw, err := os.ReadFile(abs)
	if err != nil {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	page := string(raw)
	base := `<base href="` + prefix + `">`
	if i := strings.Index(strings.ToLower(page), "<head>"); i >= 0 {
		page = page[:i+len("<head>")] + base + page[i+len("<head>"):]
	} else if i := strings.Index(strings.ToLower(page), "<html>"); i >= 0 {
		page = page[:i+len("<html>")] + "<head>" + base + "</head>" + page[i+len("<html>"):]
	} else {
		page = "<head>" + base + "</head>" + page
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, filepath.Base(abs), time.Time{}, strings.NewReader(page))
}
