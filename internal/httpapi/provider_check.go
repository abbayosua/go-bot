package httpapi

import (
	"net/http"
	"strings"

	"gobot/internal/llm"
)

// checkProvider probes the configured (or overridden) model endpoint: it must
// stream and emit the probe tool call, else setup fails with a named reason.
func (s *Server) checkProvider(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BaseURL string `json:"baseUrl"`
		Model   string `json:"model"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	baseURL := s.Provider.BaseURL
	if strings.TrimSpace(body.BaseURL) != "" {
		baseURL = strings.TrimSpace(body.BaseURL)
	}
	model := s.Provider.Model
	if strings.TrimSpace(body.Model) != "" {
		model = strings.TrimSpace(body.Model)
	}
	if baseURL == "" || model == "" {
		writeError(w, http.StatusBadRequest, "baseUrl and model are required")
		return
	}
	client := llm.New(baseURL, s.Provider.APIKey)
	result := llm.Check(r.Context(), client, model)
	if !result.OK {
		writeJSON(w, http.StatusBadGateway, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// setProvider stores the provider connection after it passes the check.
func (s *Server) setProvider(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BaseURL string `json:"baseUrl"`
		Model   string `json:"model"`
		APIKey  string `json:"apiKey"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.BaseURL) == "" || strings.TrimSpace(body.Model) == "" {
		writeError(w, http.StatusBadRequest, "baseUrl and model are required")
		return
	}
	key := s.Provider.APIKey
	if body.APIKey != "" {
		key = body.APIKey
	}
	result := llm.Check(r.Context(), llm.New(body.BaseURL, key), body.Model)
	if !result.OK {
		writeJSON(w, http.StatusBadGateway, result)
		return
	}
	if err := s.Store.SetSetting(r.Context(), "provider_base_url", strings.TrimSpace(body.BaseURL)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.Store.SetSetting(r.Context(), "provider_model", strings.TrimSpace(body.Model)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if body.APIKey != "" {
		if err := s.Store.SetSetting(r.Context(), "provider_api_key", body.APIKey); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.Provider.APIKey = body.APIKey
	}
	s.Provider.BaseURL = strings.TrimSpace(body.BaseURL)
	s.Provider.Model = strings.TrimSpace(body.Model)
	s.audit(r, "provider.set", s.Provider.Model, "")
	writeJSON(w, http.StatusOK, result)
}
