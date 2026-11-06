package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"gobot/internal/mcp"
)

// listMCPServers returns every registered MCP server with live health.
func (s *Server) listMCPServers(w http.ResponseWriter, r *http.Request) {
	servers, err := s.Store.ListMCPServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type view struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Command string `json:"command,omitempty"`
		URL     string `json:"url,omitempty"`
		Health  string `json:"health"`
		Tools   int    `json:"tools"`
	}
	out := make([]view, 0, len(servers))
	for _, server := range servers {
		item := view{ID: server.ID, Name: server.Name, Command: server.Command, URL: server.URL, Health: "unknown"}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		client := mcp.NewClient(mcp.Server{Name: server.Name, Command: server.Command, URL: server.URL, Headers: server.Headers})
		tools, err := client.ListTools(ctx)
		cancel()
		if err != nil {
			item.Health = "down: " + firstLine(err.Error())
		} else {
			health, herr := client.Health(r.Context())
			if herr != nil {
				item.Health = "up (tools listed)"
			} else {
				item.Health = health
			}
			item.Tools = len(tools)
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

// addMCPServer registers one stdio or HTTP server after probing it.
func (s *Server) addMCPServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string            `json:"name"`
		Command string            `json:"command"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if strings.TrimSpace(body.Command) == "" && strings.TrimSpace(body.URL) == "" {
		writeError(w, http.StatusBadRequest, "command or url is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	probe := mcp.NewClient(mcp.Server{Name: name, Command: body.Command, URL: body.URL, Headers: body.Headers})
	if _, err := probe.ListTools(ctx); err != nil {
		writeError(w, http.StatusBadGateway, "server probe failed: "+firstLine(err.Error()))
		return
	}
	server, err := s.Store.AddMCPServer(r.Context(), name, strings.TrimSpace(body.Command), strings.TrimSpace(body.URL), body.Headers)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "mcp.add", server.ID, name)
	writeJSON(w, http.StatusCreated, server)
}

// deleteMCPServer removes one server; its tools vanish with it.
func (s *Server) deleteMCPServer(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteMCPServer(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "mcp.delete", r.PathValue("id"), "")
	w.WriteHeader(http.StatusNoContent)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
