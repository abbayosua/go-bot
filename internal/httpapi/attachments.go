package httpapi

import (
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

// uploadAttachment stores one multipart file encrypted; only the uploader's
// session can download it again.
func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("id")
	if _, err := s.Store.GetThread(r.Context(), threadID); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	if err := r.ParseMultipartForm(10<<20 + 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "unreadable upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "field file is required")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 10<<20+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "unreadable upload")
		return
	}
	filename := filepath.Base(header.Filename)
	if strings.TrimSpace(filename) == "" || filename == "." {
		filename = "upload.bin"
	}
	attachment, err := s.Store.SaveAttachment(r.Context(), threadID,
		s.auditActor(r), filename, header.Header.Get("Content-Type"), content)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "attachment.upload", attachment.ID, filename)
	writeJSON(w, http.StatusCreated, attachment)
}

// downloadAttachment serves one blob to its uploading session only.
func (s *Server) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	attachment, content, err := s.Store.OpenAttachment(r.Context(), r.PathValue("id"), s.auditActor(r))
	if err != nil {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}
	w.Header().Set("Content-Type", attachment.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename="+attachment.Filename)
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	_, _ = w.Write(content)
}

// listAttachments returns one thread's attachment metadata.
func (s *Server) listAttachments(w http.ResponseWriter, r *http.Request) {
	attachments, err := s.Store.ListAttachments(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, attachments)
}
