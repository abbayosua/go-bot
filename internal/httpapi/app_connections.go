package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// listAppConnections returns one bot's connections (metadata only).
func (s *Server) listAppConnections(w http.ResponseWriter, r *http.Request) {
	conns, err := s.Store.ListAppConnections(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, conns)
}

// connectApp stores tokens in the vault and metadata in the row.
func (s *Server) connectApp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Provider string `json:"provider"`
		Access   string `json:"access"`
		Refresh  string `json:"refresh"`
		TokenURL string `json:"tokenUrl"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	conn, err := s.Store.ConnectApp(r.Context(), r.PathValue("id"), body.Name, body.Provider, body.Access, body.Refresh, strings.TrimSpace(body.TokenURL), 0)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "app.connect", body.Name, body.Provider)
	writeJSON(w, http.StatusCreated, conn)
}

// disconnectApp removes a connection and its vault tokens.
func (s *Server) disconnectApp(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteAppConnection(r.Context(), r.PathValue("name")); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "app.disconnect", r.PathValue("name"), "")
	w.WriteHeader(http.StatusNoContent)
}

// refreshAppConnection exchanges the stored refresh token at the token URL
// and stores the new access token: refresh with no user action.
func (s *Server) refreshAppConnection(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	conn, botID, err := s.Store.FindAppConnection(r.Context(), name)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	if conn.TokenURL == "" {
		writeError(w, http.StatusBadRequest, "no token url for this connection")
		return
	}
	refresh, err := s.Store.AppRefreshToken(r.Context(), name)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"grant_type": "refresh_token", "refresh_token": refresh,
	})
	callCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, conn.TokenURL, bytes.NewReader(payload))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, "token endpoint refused")
		return
	}
	var tokens struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if err := json.Unmarshal(raw, &tokens); err != nil || tokens.Access == "" {
		writeError(w, http.StatusBadGateway, "token endpoint gave no token")
		return
	}
	if _, err := s.Store.ConnectApp(r.Context(), botID, name, conn.Provider, tokens.Access, orKeep(refresh, tokens.Refresh), conn.TokenURL, 0); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "app.refresh", name, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "refreshed"})
}

func orKeep(old, next string) string {
	if strings.TrimSpace(next) == "" {
		return old
	}
	return next
}
