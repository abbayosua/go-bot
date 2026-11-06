package httpapi

import (
	"net/http"
	"strings"
)

// listXAgents returns registered outside services.
func (s *Server) listXAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.Store.ListExternalAgents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

// addXAgent registers one outside service by URL.
func (s *Server) addXAgent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.URL) == "" {
		writeError(w, http.StatusBadRequest, "name and url are required")
		return
	}
	if !strings.HasPrefix(body.URL, "http://") && !strings.HasPrefix(body.URL, "https://") {
		writeError(w, http.StatusBadRequest, "url must be http(s)")
		return
	}
	agent, err := s.Store.AddExternalAgent(r.Context(), strings.TrimSpace(body.Name), strings.TrimSpace(body.URL))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "xagent.add", agent.ID, agent.Name)
	writeJSON(w, http.StatusCreated, agent)
}

// deleteXAgent removes one outside service.
func (s *Server) deleteXAgent(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteExternalAgent(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "xagent.delete", r.PathValue("id"), "")
	w.WriteHeader(http.StatusNoContent)
}

// xagentCallback receives an outside result: /api/xagents/in/:token.
func (s *Server) xagentCallback(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token  string `json:"token"`
		Result string `json:"result"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" {
		token = r.PathValue("token")
	}
	task, err := s.Store.CompleteExternalTask(r.Context(), token, body.Result)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "xagent.result", task.ID, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": task.Status})
}
