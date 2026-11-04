package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gobot/internal/agent"
	"gobot/internal/bus"
	"gobot/internal/config"
	"gobot/internal/schedule"
	"gobot/internal/store"
)

type Server struct {
	Store    *store.Store
	Engine   *agent.Engine
	Bus      *bus.Bus
	Provider config.Provider
	Token    string
	Log      *slog.Logger
	Assets   fs.FS
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("POST /api/bots", s.createBot)
	mux.HandleFunc("PATCH /api/bots/{id}", s.updateBot)
	mux.HandleFunc("GET /api/bots/{id}/threads", s.listThreads)
	mux.HandleFunc("POST /api/bots/{id}/threads", s.createThread)
	mux.HandleFunc("GET /api/bots/{id}/routines", s.listRoutines)
	mux.HandleFunc("POST /api/bots/{id}/routines", s.createRoutine)
	mux.HandleFunc("DELETE /api/routines/{id}", s.deleteRoutine)
	mux.HandleFunc("GET /api/threads/{id}", s.getThread)
	mux.HandleFunc("POST /api/threads/{id}/messages", s.postMessage)
	mux.HandleFunc("GET /api/threads/{id}/events", s.events)
	mux.HandleFunc("POST /api/runs/{id}/cancel", s.cancelRun)
	mux.HandleFunc("POST /api/approvals/{id}", s.decideApproval)
	if s.Assets != nil {
		mux.Handle("GET /", http.FileServerFS(s.Assets))
	}
	return s.withAuth(mux)
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Token == "" || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == "" {
			provided = r.URL.Query().Get("token")
		}
		if provided != s.Token {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	bots, err := s.Store.ListBots(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type botView struct {
		store.Bot
		Threads []store.Thread `json:"threads"`
	}
	view := make([]botView, 0, len(bots))
	for _, bot := range bots {
		threads, err := s.Store.ListThreads(r.Context(), bot.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		view = append(view, botView{Bot: bot, Threads: threads})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bots":     view,
		"approval": s.Engine.ApprovalMode(r.Context()),
		"model":    s.Provider.Model,
	})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"approval": s.Engine.ApprovalMode(r.Context())})
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Approval string `json:"approval"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	switch body.Approval {
	case config.ApprovalAsk, config.ApprovalAuto, config.ApprovalNever:
	default:
		writeError(w, http.StatusBadRequest, "approval must be ask, auto or never")
		return
	}
	if err := s.Store.SetSetting(r.Context(), "approval_mode", body.Approval); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"approval": body.Approval})
}

func (s *Server) createBot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string `json:"name"`
		Instructions string `json:"instructions"`
		Model        string `json:"model"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	bot, err := s.Store.CreateBot(r.Context(), strings.TrimSpace(body.Name), body.Instructions, body.Model)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	thread, err := s.Store.CreateThread(r.Context(), bot.ID, "Chat")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"bot": bot, "thread": thread})
}

func (s *Server) updateBot(w http.ResponseWriter, r *http.Request) {
	bot, err := s.Store.GetBot(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	var body struct {
		Name         *string `json:"name"`
		Instructions *string `json:"instructions"`
		Model        *string `json:"model"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.Name != nil {
		bot.Name = *body.Name
	}
	if body.Instructions != nil {
		bot.Instructions = *body.Instructions
	}
	if body.Model != nil {
		bot.Model = *body.Model
	}
	if err := s.Store.UpdateBot(r.Context(), bot.ID, bot.Name, bot.Instructions, bot.Model); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, bot)
}

func (s *Server) listThreads(w http.ResponseWriter, r *http.Request) {
	threads, err := s.Store.ListThreads(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, threads)
}

func (s *Server) createThread(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		body.Title = "Chat"
	}
	thread, err := s.Store.CreateThread(r.Context(), r.PathValue("id"), body.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, thread)
}

func (s *Server) getThread(w http.ResponseWriter, r *http.Request) {
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
	runs, err := s.Store.ListRuns(r.Context(), thread.ID, 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	approvals, err := s.Store.ListApprovals(r.Context(), thread.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"thread": thread, "messages": messages, "runs": runs, "approvals": approvals,
	})
}

func (s *Server) postMessage(w http.ResponseWriter, r *http.Request) {
	thread, err := s.Store.GetThread(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	message, err := s.Store.AddMessage(r.Context(), store.Message{
		ThreadID: thread.ID, Role: "user", Content: body.Content,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Bus.Publish(thread.ID, bus.Event{Type: "message", Data: message})
	run, err := s.Store.CreateRun(r.Context(), thread.BotID, thread.ID, "user", body.Content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Engine.Start(run.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{"message": message, "run": run})
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.Store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	if err := s.Store.UpdateRun(r.Context(), run.ID, store.RunCancelled, "", run.Steps); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": store.RunCancelled})
}

func (s *Server) decideApproval(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Decision string `json:"decision"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.Decision != store.ApprovalAllow && body.Decision != store.ApprovalDeny {
		writeError(w, http.StatusBadRequest, "decision must be allow or deny")
		return
	}
	approval, err := s.Store.DecideApproval(r.Context(), r.PathValue("id"), body.Decision)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.Bus.Publish(approval.ThreadID, bus.Event{Type: "approval", Data: approval})
	s.Engine.Start(approval.RunID)
	writeJSON(w, http.StatusOK, approval)
}

func (s *Server) listRoutines(w http.ResponseWriter, r *http.Request) {
	routines, err := s.Store.ListRoutines(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, routines)
}

func (s *Server) createRoutine(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Prompt   string `json:"prompt"`
		Cron     string `json:"cron"`
		ThreadID string `json:"threadId"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	botID := r.PathValue("id")
	if strings.TrimSpace(body.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	parsed, err := schedule.Parse(body.Cron)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cron: "+err.Error())
		return
	}
	next, err := parsed.Next(time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, "cron: "+err.Error())
		return
	}
	threadID := body.ThreadID
	if threadID == "" {
		thread, err := s.Store.FirstThread(r.Context(), botID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		threadID = thread.ID
	}
	if strings.TrimSpace(body.Name) == "" {
		body.Name = "Routine"
	}
	routine, err := s.Store.CreateRoutine(r.Context(), botID, threadID, body.Name, body.Prompt, body.Cron, next.Unix())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, routine)
}

func (s *Server) deleteRoutine(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteRoutine(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// events streams thread updates as server-sent events.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("id")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	channel, unsubscribe := s.Bus.Subscribe(threadID)
	defer unsubscribe()
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case event := <-channel:
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read body")
		return false
	}
	if len(body) == 0 {
		return true
	}
	if err := json.Unmarshal(body, target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, context.Canceled):
		return http.StatusRequestTimeout
	default:
		return http.StatusBadRequest
	}
}
