package httpapi

import (
	"context"
	"net/http"
	"strings"

	"gobot/internal/bus"
	"gobot/internal/store"
)

// openDM returns (or creates) the shared DM thread for two bots.
func (s *Server) openDM(w http.ResponseWriter, r *http.Request) {
	var body struct {
		With string `json:"with"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	botID := r.PathValue("id")
	if strings.TrimSpace(body.With) == "" {
		writeError(w, http.StatusBadRequest, "with is required")
		return
	}
	peer, err := s.Store.FindBot(r.Context(), strings.TrimSpace(body.With))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	dm, thread, err := s.Store.OpenDM(r.Context(), botID, peer.ID)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "dm.open", dm.ID, peer.Name)
	writeJSON(w, http.StatusOK, map[string]any{"dm": dm, "thread": thread})
}

// listDMs returns one bot's DM threads.
func (s *Server) listDMs(w http.ResponseWriter, r *http.Request) {
	dms, err := s.Store.ListDMs(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dms)
}

// sendDM posts as one bot into the pair's shared thread and starts the
// other bot's run, so the reply comes from the peer, not the sender.
func (s *Server) sendDM(w http.ResponseWriter, r *http.Request) {
	dmID := r.PathValue("id")
	var body struct {
		From    string `json:"from"`
		Content string `json:"content"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Content) == "" || strings.TrimSpace(body.From) == "" {
		writeError(w, http.StatusBadRequest, "from and content are required")
		return
	}
	dm, thread, err := s.storeDM(r.Context(), dmID)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	sender := ""
	for _, member := range dm.MemberIDs {
		if member == strings.TrimSpace(body.From) {
			sender = member
		}
	}
	if sender == "" {
		writeError(w, http.StatusBadRequest, "from is not a member of this DM")
		return
	}
	peer := dm.MemberIDs[0]
	if peer == sender {
		peer = dm.MemberIDs[1]
	}
	senderBot, err := s.Store.GetBot(r.Context(), sender)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	message, err := s.Store.AddMessage(r.Context(), store.Message{ThreadID: thread.ID, Role: "user", Content: senderBot.Name + ": " + strings.TrimSpace(body.Content)})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Bus.Publish(thread.ID, bus.Event{Type: "message", Data: message})
	run, err := s.Store.CreateRun(r.Context(), peer, thread.ID, "dm", body.Content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Engine.Start(run.ID)
	s.audit(r, "dm.send", dm.ID, senderBot.Name)
	writeJSON(w, http.StatusAccepted, map[string]any{"message": message, "run": run})
}

func (s *Server) storeDM(ctx context.Context, dmID string) (store.DM, store.Thread, error) {
	dm, err := s.Store.GetDM(ctx, dmID)
	if err != nil {
		return store.DM{}, store.Thread{}, err
	}
	thread, err := s.Store.GetThread(ctx, dm.ThreadID)
	return dm, thread, err
}
