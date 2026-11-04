package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gobot/internal/llm"
)

const (
	fetchTimeout = 30 * time.Second
	fetchLimit   = 512 * 1024
	fetchChars   = 20000
)

var (
	tagPattern    = regexp.MustCompile(`(?s)<[^>]*>`)
	scriptPattern = regexp.MustCompile(`(?is)<(script|style|noscript)\b.*?</(script|style|noscript)>`)
	spacePattern  = regexp.MustCompile(`[ \t]+`)
	blankLines    = regexp.MustCompile(`\n{3,}`)
)

type webFetchTool struct{ client *http.Client }

func (t *webFetchTool) Name() string { return "web_fetch" }

func (t *webFetchTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "web_fetch",
		Description: "Fetch an http(s) URL and return its readable text.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"url": {"type": "string", "description": "Absolute http or https URL."}
			},
			"required": ["url"]
		}`),
	}}
}

func (t *webFetchTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
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
		t.client = &http.Client{Timeout: fetchTimeout}
	}
	runCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(runCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Go-Bot/1.0 (+https://github.com/)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.5")
	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, fetchLimit))
	if err != nil {
		return "", err
	}
	text := readable(string(raw), resp.Header.Get("Content-Type"))
	if len(text) > fetchChars {
		text = text[:fetchChars] + "\n[truncated]"
	}
	return fmt.Sprintf("HTTP %d\n\n%s", resp.StatusCode, text), nil
}

func readable(body, contentType string) string {
	if !strings.Contains(strings.ToLower(contentType), "html") {
		return strings.TrimSpace(body)
	}
	cleaned := scriptPattern.ReplaceAllString(body, " ")
	cleaned = tagPattern.ReplaceAllString(cleaned, " ")
	cleaned = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"").Replace(cleaned)
	cleaned = spacePattern.ReplaceAllString(cleaned, " ")
	cleaned = blankLines.ReplaceAllString(cleaned, "\n\n")
	return strings.TrimSpace(cleaned)
}
