package httpapi

import (
	"net/http"
)

// createShare mints a revocable read-only link for a thread.
func (s *Server) createShare(w http.ResponseWriter, r *http.Request) {
	share, err := s.Store.CreateShare(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "share.create", share.ID, "")
	writeJSON(w, http.StatusCreated, map[string]string{"id": share.ID, "token": share.Token})
}

// listShares returns a thread's active links (ids only; tokens show once).
func (s *Server) listShares(w http.ResponseWriter, r *http.Request) {
	shares, err := s.Store.ListShares(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ids := make([]map[string]string, 0, len(shares))
	for _, share := range shares {
		ids = append(ids, map[string]string{"id": share.ID})
	}
	writeJSON(w, http.StatusOK, ids)
}

// revokeShare deletes one link.
func (s *Server) revokeShare(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.RevokeShare(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "share.revoke", r.PathValue("id"), "")
	w.WriteHeader(http.StatusNoContent)
}

// sharedThread serves a linked thread read-only without a server token.
func (s *Server) sharedThread(w http.ResponseWriter, r *http.Request) {
	share, err := s.Store.GetShareByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		writeError(w, http.StatusNotFound, "link revoked or unknown")
		return
	}
	thread, err := s.Store.GetThread(r.Context(), share.ThreadID)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	messages, err := s.Store.ListMessages(r.Context(), thread.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	public := make([]map[string]string, 0, len(messages))
	for _, m := range messages {
		if m.Role == "user" || m.Role == "assistant" {
			public = append(public, map[string]string{"role": m.Role, "content": m.Content})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": thread, "messages": public})
}
