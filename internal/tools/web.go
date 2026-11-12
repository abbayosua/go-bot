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

const (
	searchTimeout = 20 * time.Second
	searchResults = 10
)

type webSearchTool struct{ client *http.Client }

func (t *webSearchTool) Name() string { return "web_search" }

func (t *webSearchTool) Spec() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        "web_search",
		Description: "Search the web without an API key (DuckDuckGo instant answers plus Wikipedia) and return titles, URLs and snippets.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "Search query."}
			},
			"required": ["query"]
		}`),
	}}
}

func (t *webSearchTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return "", errors.New("query is required")
	}
	if t.client == nil {
		t.client = &http.Client{Timeout: searchTimeout}
	}
	var out strings.Builder
	if instant := t.instantAnswer(ctx, query); instant != "" {
		out.WriteString("DuckDuckGo:\n")
		out.WriteString(instant)
		out.WriteString("\n\n")
	}
	if wiki := t.wikipedia(ctx, query); wiki != "" {
		out.WriteString("Wikipedia:\n")
		out.WriteString(wiki)
	}
	result := strings.TrimSpace(out.String())
	if result == "" {
		return "", errors.New("search returned no results")
	}
	return result, nil
}

func (t *webSearchTool) getJSON(ctx context.Context, endpoint string, target any) error {
	runCtx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(runCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Go-Bot/1.0 (local assistant; mailto:admin@localhost)")
	req.Header.Set("Accept", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, fetchLimit))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func (t *webSearchTool) instantAnswer(ctx context.Context, query string) string {
	endpoint := "https://api.duckduckgo.com/?q=" + url.QueryEscape(query) + "&format=json&no_html=1&skip_disambig=1"
	var parsed struct {
		Abstract      string `json:"Abstract"`
		AbstractURL   string `json:"AbstractURL"`
		Answer        string `json:"Answer"`
		AnswerType    string `json:"AnswerType"`
		Definition    string `json:"Definition"`
		DefinitionURL string `json:"DefinitionURL"`
		RelatedTopics []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
	}
	if err := t.getJSON(ctx, endpoint, &parsed); err != nil {
		return ""
	}
	var out strings.Builder
	if parsed.Answer != "" {
		fmt.Fprintf(&out, "- Answer (%s): %s\n", parsed.AnswerType, strings.TrimSpace(parsed.Answer))
	}
	if parsed.Abstract != "" {
		fmt.Fprintf(&out, "- %s <%s>\n", strings.TrimSpace(parsed.Abstract), parsed.AbstractURL)
	} else if parsed.Definition != "" {
		fmt.Fprintf(&out, "- %s <%s>\n", strings.TrimSpace(parsed.Definition), parsed.DefinitionURL)
	}
	count := 0
	for _, topic := range parsed.RelatedTopics {
		if strings.TrimSpace(topic.Text) == "" || strings.TrimSpace(topic.FirstURL) == "" {
			continue
		}
		fmt.Fprintf(&out, "- %s <%s>\n", strings.TrimSpace(topic.Text), topic.FirstURL)
		count++
		if count >= searchResults {
			break
		}
	}
	return strings.TrimSpace(out.String())
}

func (t *webSearchTool) wikipedia(ctx context.Context, query string) string {
	endpoint := "https://en.wikipedia.org/w/api.php?action=opensearch&search=" + url.QueryEscape(query) + "&limit=5&format=json"
	var parsed []json.RawMessage
	if err := t.getJSON(ctx, endpoint, &parsed); err != nil || len(parsed) != 4 {
		return ""
	}
	var titles, descriptions, links []string
	if err := json.Unmarshal(parsed[1], &titles); err != nil {
		return ""
	}
	if err := json.Unmarshal(parsed[2], &descriptions); err != nil {
		return ""
	}
	if err := json.Unmarshal(parsed[3], &links); err != nil {
		return ""
	}
	var out strings.Builder
	for i := range titles {
		if i >= 5 {
			break
		}
		link := ""
		if i < len(links) {
			link = links[i]
		}
		desc := ""
		if i < len(descriptions) {
			desc = descriptions[i]
		}
		fmt.Fprintf(&out, "- %s <%s> %s\n", strings.TrimSpace(titles[i]), link, strings.TrimSpace(desc))
	}
	return strings.TrimSpace(out.String())
}
