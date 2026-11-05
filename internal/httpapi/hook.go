package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
)

// inboundHook fires a routine from a signed webhook: POST /api/hooks/{routine}
// with the raw body and header X-Signature (hex HMAC-SHA256 over the body,
// keyed by the server token). The body becomes the run prompt suffix.
func (s *Server) inboundHook(w http.ResponseWriter, r *http.Request) {
	if s.Token == "" {
		writeError(w, http.StatusBadRequest, "hooks require a server token")
		return
	}
	routine, err := s.Store.GetRoutine(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "unreadable body")
		return
	}
	signature := strings.TrimSpace(r.Header.Get("X-Signature"))
	mac := hmac.New(sha256.New, []byte(s.Token))
	mac.Write(body)
	if subtle.ConstantTimeCompare([]byte(signature), []byte(hex.EncodeToString(mac.Sum(nil)))) != 1 {
		writeError(w, http.StatusUnauthorized, "bad signature")
		return
	}
	prompt := routine.Prompt
	if text := strings.TrimSpace(string(body)); text != "" {
		if len(text) > 2000 {
			text = text[:2000]
		}
		prompt += "\n\nWebhook payload:\n" + text
	}
	run, err := s.Store.CreateRun(r.Context(), routine.BotID, routine.ThreadID, "webhook", prompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "hook.fire", routine.ID, "")
	s.Engine.Start(run.ID)
	writeJSON(w, http.StatusAccepted, map[string]string{"runId": run.ID})
}
