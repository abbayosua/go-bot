package httpapi

import (
	"io"
	"net/http"
	"strings"

	"gobot/internal/bus"
	"gobot/internal/msg"
	"gobot/internal/store"
)

// listMsgInstalls returns one bot's messaging connections (tokens hidden).
func (s *Server) listMsgInstalls(w http.ResponseWriter, r *http.Request) {
	installs, err := s.Store.ListMsgInstalls(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, installs)
}

// addMsgInstall connects a bot to an adapter. The token is shown once.
func (s *Server) addMsgInstall(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Adapter string `json:"adapter"`
		Token   string `json:"token"`
		Chats   string `json:"chats"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Token) == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	thread, err := s.Store.FirstThread(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	install, err := s.Store.AddMsgInstall(r.Context(), r.PathValue("id"),
		strings.TrimSpace(body.Adapter), strings.TrimSpace(body.Token), body.Chats, thread.ID)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "msg.connect", install.ID, install.Adapter)
	// Token shown once at creation; lists never include it.
	_, raw, _ := s.Store.MsgInstallForToken(r.Context(), strings.TrimSpace(body.Token))
	_ = raw
	writeJSON(w, http.StatusCreated, install)
}

// setMsgChats replaces who may talk to the bot.
func (s *Server) setMsgChats(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Chats string `json:"chats"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.Store.SetMsgChats(r.Context(), r.PathValue("id"), body.Chats); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "msg.share", r.PathValue("id"), body.Chats)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// deleteMsgInstall removes one connection.
func (s *Server) deleteMsgInstall(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteMsgInstall(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "msg.disconnect", r.PathValue("id"), "")
	w.WriteHeader(http.StatusNoContent)
}

// inboundMsg receives a Telegram webhook: /api/msg/in/:token. Unknown chats
// and unknown people are dropped before any run starts.
func (s *Server) inboundMsg(w http.ResponseWriter, r *http.Request) {
	install, raw, err := s.Store.MsgInstallForToken(r.Context(), r.PathValue("token"))
	if err != nil {
		writeError(w, http.StatusNotFound, "unknown install")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "unreadable body")
		return
	}
	message, _ := msg.ParseUpdate(body)
	if message == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	if !store.ChatAllowed(install.Chats, message.ChatID) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "dropped"})
		return
	}
	thread, err := s.Store.GetThread(r.Context(), install.Thread)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	inbound := "[" + install.Adapter + ":" + message.From + "] " + message.Text
	stored, err := s.Store.AddMessage(r.Context(), store.Message{ThreadID: thread.ID, Role: "user", Content: inbound})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Bus.Publish(thread.ID, bus.Event{Type: "message", Data: stored})
	run, err := s.Store.CreateRun(r.Context(), install.BotID, thread.ID, "msg", inbound)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Engine.Start(run.ID)
	s.audit(r, "msg.inbound", install.ID, message.ChatID)
	_ = raw
	writeJSON(w, http.StatusAccepted, map[string]string{"runId": run.ID})
}
