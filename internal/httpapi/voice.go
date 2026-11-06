package httpapi

import (
	"io"
	"net/http"
	"strings"

	"gobot/internal/llm"
)

// dictate accepts one audio upload and returns its transcript. Providers
// without transcription fail openly with 502 so the UI says unavailable.
func (s *Server) dictate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8<<20 + 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "unreadable upload")
		return
	}
	file, header, err := r.FormFile("audio")
	if err != nil {
		writeError(w, http.StatusBadRequest, "field audio is required")
		return
	}
	defer file.Close()
	audio, err := io.ReadAll(io.LimitReader(file, 8<<20+1))
	if err != nil || len(audio) == 0 {
		writeError(w, http.StatusBadRequest, "unreadable upload")
		return
	}
	text, err := llm.New(s.Provider.BaseURL, s.Provider.APIKey).Transcribe(r.Context(), s.VoiceModel(), header.Filename, audio)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
}

// speakReply synthesizes the latest assistant message in a thread.
func (s *Server) speakReply(w http.ResponseWriter, r *http.Request) {
	thread, err := s.Store.GetThread(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	messages, err := s.Store.ListMessages(r.Context(), thread.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	text := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && strings.TrimSpace(messages[i].Content) != "" {
			text = messages[i].Content
			break
		}
	}
	if text == "" {
		writeError(w, http.StatusNotFound, "no assistant reply to speak")
		return
	}
	audio, contentType, err := llm.New(s.Provider.BaseURL, s.Provider.APIKey).Speak(r.Context(), s.VoiceModel(), "alloy", text)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(audio)
}

// VoiceModel picks the transcription/speech model, falling back to chat.
func (s *Server) VoiceModel() string {
	if s.Provider.VoiceModel != "" {
		return s.Provider.VoiceModel
	}
	return s.Provider.Model
}
