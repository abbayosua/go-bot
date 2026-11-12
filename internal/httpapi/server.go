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
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		stats, err := s.Store.Stats(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "queue": stats})
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
	mux.HandleFunc("PATCH /api/routines/{id}", s.updateRoutine)
	mux.HandleFunc("PUT /api/routines/{id}/watch", s.setRoutineWatch)
	mux.HandleFunc("POST /api/provider/check", s.checkProvider)
	mux.HandleFunc("PUT /api/provider", s.setProvider)
	mux.HandleFunc("GET /api/audit", s.listAudit)
	mux.HandleFunc("GET /api/bots/{id}/board", s.listBoard)
	mux.HandleFunc("POST /api/bots/{id}/board", s.createBoardCard)
	mux.HandleFunc("PATCH /api/board/{id}", s.moveBoardCard)
	mux.HandleFunc("DELETE /api/board/{id}", s.deleteBoardCard)
	mux.HandleFunc("GET /api/threads/{id}/artifacts", s.listArtifacts)
	mux.HandleFunc("GET /api/threads/{id}/artifact", s.downloadArtifact)
	mux.HandleFunc("POST /api/hooks/{id}", s.inboundHook)
	mux.HandleFunc("POST /api/threads/{id}/shares", s.createShare)
	mux.HandleFunc("GET /api/threads/{id}/shares", s.listShares)
	mux.HandleFunc("DELETE /api/shares/{id}", s.revokeShare)
	mux.HandleFunc("POST /api/agents/import", s.importAgent)
	mux.HandleFunc("GET /api/shared/{token}", s.sharedThread)
	mux.HandleFunc("POST /api/threads/{id}/attachments", s.uploadAttachment)
	mux.HandleFunc("GET /api/threads/{id}/attachments", s.listAttachments)
	mux.HandleFunc("GET /api/attachments/{id}", s.downloadAttachment)
	mux.HandleFunc("POST /api/groups", s.createGroup)
	mux.HandleFunc("GET /api/groups", s.listGroups)
	mux.HandleFunc("GET /api/bots/{id}/fleet", s.getFleetSwitch)
	mux.HandleFunc("PUT /api/bots/{id}/fleet", s.setFleetSwitch)
	mux.HandleFunc("POST /api/bots/{id}/dms", s.openDM)
	mux.HandleFunc("GET /api/bots/{id}/dms", s.listDMs)
	mux.HandleFunc("POST /api/dms/{id}/messages", s.sendDM)
	mux.HandleFunc("GET /api/mcp/servers", s.listMCPServers)
	mux.HandleFunc("POST /api/mcp/servers", s.addMCPServer)
	mux.HandleFunc("DELETE /api/mcp/servers/{id}", s.deleteMCPServer)
	mux.HandleFunc("GET /api/bots/{id}/msg", s.listMsgInstalls)
	mux.HandleFunc("POST /api/bots/{id}/msg", s.addMsgInstall)
	mux.HandleFunc("PUT /api/msg/{id}", s.setMsgChats)
	mux.HandleFunc("DELETE /api/msg/{id}", s.deleteMsgInstall)
	mux.HandleFunc("POST /api/voice/dictate", s.dictate)
	mux.HandleFunc("GET /api/threads/{id}/speak", s.speakReply)
	mux.HandleFunc("GET /api/bots/{id}/plugins", s.listPlugins)
	mux.HandleFunc("POST /api/bots/{id}/plugins", s.installPlugin)
	mux.HandleFunc("PUT /api/plugins/{id}", s.setPluginEnabled)
	mux.HandleFunc("DELETE /api/plugins/{id}", s.deletePlugin)
	mux.HandleFunc("POST /api/users", s.register)
	mux.HandleFunc("POST /api/session", s.login)
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("DELETE /api/session", s.logout)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.revokeSession)
	mux.HandleFunc("GET /api/users", s.listUsers)
	mux.HandleFunc("PUT /api/users/{id}", s.setUserRole)
	mux.HandleFunc("PUT /api/spaces/{id}/roles", s.setSpaceRole)
	mux.HandleFunc("GET /api/bots/{id}/apps", s.listAppConnections)
	mux.HandleFunc("POST /api/bots/{id}/apps", s.connectApp)
	mux.HandleFunc("DELETE /api/apps/{name}", s.disconnectApp)
	mux.HandleFunc("POST /api/apps/{name}/refresh", s.refreshAppConnection)
	mux.HandleFunc("GET /api/bots/{id}/wallet", s.walletBalance)
	mux.HandleFunc("POST /api/bots/{id}/wallet/topup", s.walletTopUp)
	mux.HandleFunc("POST /api/bots/{id}/wallet/charge", s.walletCharge)
	mux.HandleFunc("GET /api/xagents", s.listXAgents)
	mux.HandleFunc("POST /api/xagents", s.addXAgent)
	mux.HandleFunc("DELETE /api/xagents/{id}", s.deleteXAgent)
	mux.HandleFunc("POST /api/xagents/in/{token}", s.xagentCallback)
	mux.HandleFunc("POST /api/msg/in/{token}", s.inboundMsg)
	mux.HandleFunc("GET /api/secrets", s.listSecrets)
	mux.HandleFunc("POST /api/secrets", s.setSecret)
	mux.HandleFunc("DELETE /api/secrets/{name}", s.deleteSecret)
	mux.HandleFunc("GET /api/spaces", s.listSpaces)
	mux.HandleFunc("POST /api/spaces", s.createSpace)
	mux.HandleFunc("DELETE /api/spaces/{id}", s.deleteSpace)
	mux.HandleFunc("GET /api/backup", s.exportAll)
	mux.HandleFunc("POST /api/backup", s.importAll)
	mux.HandleFunc("GET /api/routines/{id}/runs", s.routineRuns)
	mux.HandleFunc("GET /api/search", s.search)
	mux.HandleFunc("GET /api/bots/{id}/memories", s.listMemories)
	mux.HandleFunc("DELETE /api/memories/{id}", s.deleteMemory)
	mux.HandleFunc("GET /api/bots/{id}/knowledge", s.listKnowledge)
	mux.HandleFunc("POST /api/bots/{id}/knowledge", s.createKnowledge)
	mux.HandleFunc("DELETE /api/knowledge/{id}", s.deleteKnowledge)
	mux.HandleFunc("GET /api/skills", s.listSkills)
	mux.HandleFunc("POST /api/skills", s.createSkill)
	mux.HandleFunc("DELETE /api/skills/{id}", s.deleteSkill)
	mux.HandleFunc("GET /api/bots/{id}/instructions", s.listInstructions)
	mux.HandleFunc("POST /api/bots/{id}/instructions", s.proposeInstructions)
	mux.HandleFunc("POST /api/instructions/{id}/apply", s.applyInstructions)
	mux.HandleFunc("POST /api/instructions/{id}/undo", s.undoInstructions)
	mux.HandleFunc("GET /api/rules", s.listRules)
	mux.HandleFunc("POST /api/rules", s.createRule)
	mux.HandleFunc("DELETE /api/rules/{id}", s.deleteRule)
	mux.HandleFunc("GET /api/threads/{id}", s.getThread)
	mux.HandleFunc("POST /api/threads/{id}/messages", s.postMessage)
	mux.HandleFunc("GET /api/threads/{id}/events", s.events)
	mux.HandleFunc("GET /api/threads/{id}/feed", s.feed)
	mux.HandleFunc("POST /api/runs/{id}/cancel", s.cancelRun)
	mux.HandleFunc("POST /api/approvals/{id}", s.decideApproval)
	mux.HandleFunc("GET /api/bots/{id}/scratchpad", s.getScratchpad)
	mux.HandleFunc("POST /api/bots/{id}/scratchpad", s.addScratchpad)
	mux.HandleFunc("DELETE /api/bots/{id}/scratchpad", s.clearScratchpad)
	if s.Assets != nil {
		mux.Handle("GET /", http.FileServerFS(s.Assets))
	}
	return s.withAuth(mux)
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/api/shared/") || strings.HasPrefix(path, "/api/msg/in/") || strings.HasPrefix(path, "/api/xagents/in/") ||
			path == "/api/health" || path == "/api/session" || (path == "/api/users" && r.Method == http.MethodPost) {
			next.ServeHTTP(w, r)
			return
		}
		if s.Token == "" || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == "" {
			provided = r.URL.Query().Get("token")
		}
		if user, _, ok := s.currentUser(r); ok {
			_ = user
			next.ServeHTTP(w, r)
			return
		}
		if provided != s.Token {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	spaces, err := s.Store.ListSpaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	spaceFilter := strings.TrimSpace(r.URL.Query().Get("spaceId"))
	var bots []store.Bot
	if spaceFilter != "" {
		bots, err = s.Store.ListBotsIn(r.Context(), spaceFilter)
	} else {
		bots, err = s.Store.ListBots(r.Context())
	}
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
		"spaces":   spaces,
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
		SpaceID      string `json:"spaceId"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	bot, err := s.Store.CreateBotIn(r.Context(), strings.TrimSpace(body.SpaceID), strings.TrimSpace(body.Name), body.Instructions, body.Model)
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
		"usage": summarizeUsage(messages),
	})
}

// summarizeUsage aggregates prompt/cached tokens across assistant messages so
// the UI can show one cache-hit percentage per chat session.
func summarizeUsage(messages []store.Message) map[string]any {
	var prompt, cached int
	for _, m := range messages {
		prompt += m.PromptTokens
		cached += m.CachedTokens
	}
	var pct float64
	if prompt > 0 {
		pct = float64(cached) / float64(prompt) * 100
	}
	return map[string]any{"promptTokens": prompt, "cachedTokens": cached, "cachePct": pct}
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
	group, err := s.Store.GetGroupByThread(r.Context(), thread.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if group == nil {
		run, err := s.Store.CreateRun(r.Context(), thread.BotID, thread.ID, "user", body.Content)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.Engine.Start(run.ID)
		writeJSON(w, http.StatusAccepted, map[string]any{"message": message, "run": run})
		return
	}
	// Group chat: @mentions route the turn; a bare message asks everyone.
	members := group.MemberIDs
	if mentioned := mentionMembers(body.Content, s.memberNames(r.Context(), group)); len(mentioned) > 0 {
		members = mentioned
	}
	runs := make([]store.Run, 0, len(members))
	for _, memberID := range members {
		run, err := s.Store.CreateRun(r.Context(), memberID, thread.ID, "user", body.Content)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.Engine.Start(run.ID)
		runs = append(runs, run)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"message": message, "runs": runs})
}

// memberNames resolves member ids to display names for mention matching.
func (s *Server) memberNames(ctx context.Context, group *store.Group) map[string]string {
	names := map[string]string{}
	for _, id := range group.MemberIDs {
		if bot, err := s.Store.GetBot(ctx, id); err == nil {
			names[id] = bot.Name
		}
	}
	return names
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
	s.audit(r, "run.cancel", r.PathValue("id"), "")
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
	s.audit(r, "approval.decide", r.PathValue("id"), body.Decision)
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
	s.audit(r, "routine.delete", r.PathValue("id"), "")
	if err := s.Store.DeleteRoutine(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// routineRuns returns the recent scheduled runs for one routine, newest
// first: the routine's run history.
func (s *Server) routineRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.Store.ListRoutineRuns(r.Context(), r.PathValue("id"), 10)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

// search finds messages across every bot, so one query can surface context
// from different conversations.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	matches, err := s.Store.SearchMessages(r.Context(), query, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, matches)
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
	if err != nil && (err.Error() == "status must be todo, doing or done" || err.Error() == "space still holds bots" || err.Error() == "a bot cannot DM itself" || err.Error() == "groups hold 2-10 bots" || err.Error() == "every member must live in the group space") {
		return http.StatusBadRequest
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, context.Canceled):
		return http.StatusRequestTimeout
	default:
		return http.StatusBadRequest
	}
}

func (s *Server) listMemories(w http.ResponseWriter, r *http.Request) {
	memories, err := s.Store.ListMemories(r.Context(), r.PathValue("id"), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, memories)
}

func (s *Server) deleteMemory(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteMemory(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listKnowledge(w http.ResponseWriter, r *http.Request) {
	entries, err := s.Store.ListKnowledge(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) createKnowledge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Title) == "" || strings.TrimSpace(body.Body) == "" {
		writeError(w, http.StatusBadRequest, "title and body are required")
		return
	}
	entry, err := s.Store.AddKnowledge(r.Context(), r.PathValue("id"), strings.TrimSpace(body.Title), body.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (s *Server) deleteKnowledge(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteKnowledge(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	skills, err := s.Store.ListSkills(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, skills)
}

func (s *Server) createSkill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Body        string `json:"body"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	name := strings.ToLower(strings.Join(strings.Fields(body.Name), "-"))
	if name == "" || strings.TrimSpace(body.Body) == "" {
		writeError(w, http.StatusBadRequest, "name and body are required")
		return
	}
	skill, err := s.Store.CreateSkill(r.Context(), name, body.Description, body.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, skill)
}

func (s *Server) deleteSkill(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteSkill(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listInstructions(w http.ResponseWriter, r *http.Request) {
	bot, err := s.Store.GetBot(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	versions, err := s.Store.ListInstructionVersions(r.Context(), bot.ID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"instructions": bot.Instructions, "versions": versions})
}

func (s *Server) proposeInstructions(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Instructions string `json:"instructions"`
		Source       string `json:"source"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Instructions) == "" {
		writeError(w, http.StatusBadRequest, "instructions are required")
		return
	}
	if body.Source == "" {
		body.Source = "user"
	}
	version, err := s.Store.AddInstructionVersion(r.Context(), r.PathValue("id"), strings.TrimSpace(body.Instructions), body.Source)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, version)
}

func (s *Server) applyInstructions(w http.ResponseWriter, r *http.Request) {
	version, err := s.instructionVersion(r, r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	bot, err := s.Store.GetBot(r.Context(), version.BotID)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	if err := s.Store.UpdateBot(r.Context(), bot.ID, bot.Name, version.Instructions, bot.Model); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"instructions": version.Instructions})
}

func (s *Server) undoInstructions(w http.ResponseWriter, r *http.Request) {
	version, err := s.instructionVersion(r, r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	previous, err := s.Store.UndoInstructionVersion(r.Context(), version.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	bot, err := s.Store.GetBot(r.Context(), version.BotID)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	if err := s.Store.UpdateBot(r.Context(), bot.ID, bot.Name, previous, bot.Model); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"instructions": previous})
}

func (s *Server) instructionVersion(r *http.Request, id string) (store.InstructionVersion, error) {
	versions, err := s.Store.ListInstructionVersions(r.Context(), "", 1000)
	if err != nil {
		return store.InstructionVersion{}, err
	}
	for _, version := range versions {
		if version.ID == id {
			return version, nil
		}
	}
	return store.InstructionVersion{}, store.ErrNotFound
}

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	botID := r.URL.Query().Get("botId")
	rules, err := s.Store.ListApprovalRules(r.Context(), botID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Scope string `json:"scope"`
		Kind  string `json:"kind"`
		Tool  string `json:"tool"`
		Args  string `json:"args"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.Kind != "allow" && body.Kind != "deny" {
		writeError(w, http.StatusBadRequest, "kind must be allow or deny")
		return
	}
	rule, err := s.Store.AddApprovalRule(r.Context(), strings.TrimSpace(body.Scope), body.Kind, strings.TrimSpace(body.Tool), strings.TrimSpace(body.Args))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteApprovalRule(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) feed(w http.ResponseWriter, r *http.Request) {
	messages, err := s.Store.ListMessages(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, store.Feed(messages))
}
