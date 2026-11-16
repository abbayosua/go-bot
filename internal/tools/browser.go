package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gobot/internal/llm"
)

// browserSidecarURL is the local Scrapling sidecar (scripts/scrapling-sidecar.py).
// StealthyFetcher renders JS + beats Turnstile/captcha where web_fetch fails.
func browserSidecarURL() string {
	if u := strings.TrimSpace(os.Getenv("GOBOT_BROWSER_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://127.0.0.1:8765"
}

type browserFetchTool struct{ client *http.Client }

func (t *browserFetchTool) Name() string { return "browser_fetch" }

func (t *browserFetchTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "browser_fetch",
		Description: "Fetch an http(s) URL through a real Chromium (StealthyFetcher): renders JavaScript and bypasses Turnstile/captcha where web_fetch fails. Returns headings and paragraph text.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"url": {"type": "string", "description": "Absolute http or https URL."}
			},
			"required": ["url"]
		}`),
	}}
}

func (t *browserFetchTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	parsed, err := url.Parse(strings.TrimSpace(input.URL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("url must be an absolute http or https URL")
	}
	if t.client == nil {
		t.client = &http.Client{Timeout: 120 * time.Second}
	}
	raw, err := json.Marshal(map[string]string{"url": strings.TrimSpace(input.URL)})
	if err != nil {
		return "", err
	}
	runCtx, cancel := context.WithTimeout(ctx, 115*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(runCtx, http.MethodPost, browserSidecarURL()+"/fetch", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("browser sidecar unreachable (start scripts/scrapling-sidecar.py): %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, fetchLimit))
	if err != nil {
		return "", err
	}
	var out struct {
		Status int    `json:"status"`
		Text   string `json:"text"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("browser sidecar bad response %d: %.200s", resp.StatusCode, body)
	}
	if resp.StatusCode >= 400 || out.Error != "" {
		return "", fmt.Errorf("browser_fetch: %s", out.Error)
	}
	text := strings.TrimSpace(out.Text)
	if text == "" {
		return "", errors.New("browser_fetch returned no readable text")
	}
	if len(text) > fetchChars {
		text = text[:fetchChars] + "\n[truncated]"
	}
	return fmt.Sprintf("[HTTP %d via Chromium]\n%s", out.Status, text), nil
}
