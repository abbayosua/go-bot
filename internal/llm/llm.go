package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message is one OpenAI-compatible chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type Result struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        Usage
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	CachedTokens     int `json:"cached_tokens,omitempty"`
}

func (u *Usage) UnmarshalJSON(data []byte) error {
	type raw Usage
	var plain raw
	if err := json.Unmarshal(data, &plain); err != nil {
		return err
	}
	*u = Usage(plain)
	var details struct {
		PromptDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	}
	if err := json.Unmarshal(data, &details); err == nil && details.PromptDetails != nil {
		u.CachedTokens = details.PromptDetails.CachedTokens
	}
	return nil
}

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{BaseURL: strings.TrimSuffix(baseURL, "/"), APIKey: apiKey, HTTP: &http.Client{Timeout: 0}}
}

type request struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	Tools          []Tool    `json:"tools,omitempty"`
	Stream         bool      `json:"stream"`
	PromptCacheKey *string   `json:"prompt_cache_key,omitempty"`
	StreamOptions  *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Stream sends one chat completion and calls onText for each text delta. Tool
// call fragments are reassembled by index before returning.
func (c *Client) Stream(ctx context.Context, model string, messages []Message, tools []Tool, onText func(string)) (Result, error) {
	return c.StreamWithCache(ctx, model, messages, tools, "", onText)
}

// StreamWithCache is Stream with an optional provider prompt-cache key. Empty
// means no caching: every token is billed fresh.
func (c *Client) StreamWithCache(ctx context.Context, model string, messages []Message, tools []Tool, cacheKey string, onText func(string)) (Result, error) {
	body := request{Model: model, Messages: messages, Tools: tools, Stream: true}
	if strings.TrimSpace(cacheKey) != "" {
		key := strings.TrimSpace(cacheKey)
		body.PromptCacheKey = &key
	}
	body.StreamOptions = &struct {
		IncludeUsage bool `json:"include_usage"`
	}{IncludeUsage: true}
	payload, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Result{}, fmt.Errorf("model provider returned %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}

	var (
		content   strings.Builder
		calls     []ToolCall
		finish    string
		usage     Usage
		reader    = bufio.NewReader(resp.Body)
		sawStream bool
	)
	for {
		line, readErr := reader.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if line != "" {
			if data, ok := strings.CutPrefix(line, "data:"); ok {
				sawStream = true
				data = strings.TrimSpace(data)
				if data != "[DONE]" {
					var chunk streamChunk
					if err := json.Unmarshal([]byte(data), &chunk); err != nil {
						return Result{}, fmt.Errorf("decode stream chunk: %w", err)
					}
					if chunk.Error != nil {
						return Result{}, errors.New(chunk.Error.Message)
					}
					if chunk.Usage != nil {
						usage = *chunk.Usage
					}
					for _, choice := range chunk.Choices {
						if choice.Delta.Content != "" {
							content.WriteString(choice.Delta.Content)
							if onText != nil {
								onText(choice.Delta.Content)
							}
						}
						for _, fragment := range choice.Delta.ToolCalls {
							for len(calls) <= fragment.Index {
								calls = append(calls, ToolCall{Type: "function"})
							}
							call := &calls[fragment.Index]
							if fragment.ID != "" {
								call.ID = fragment.ID
							}
							if fragment.Type != "" {
								call.Type = fragment.Type
							}
							call.Function.Name += fragment.Function.Name
							call.Function.Arguments += fragment.Function.Arguments
						}
						if choice.FinishReason != "" {
							finish = choice.FinishReason
						}
					}
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return Result{}, readErr
		}
	}
	if !sawStream {
		return Result{}, errors.New("model provider sent no stream; check the base URL")
	}
	for i := range calls {
		if calls[i].ID == "" {
			calls[i].ID = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), i)
		}
		if calls[i].Type == "" {
			calls[i].Type = "function"
		}
	}
	return Result{Content: content.String(), ToolCalls: calls, FinishReason: finish, Usage: usage}, nil
}

// Complete is the non-streaming path, used for the approval judge.
func (c *Client) Complete(ctx context.Context, model string, messages []Message) (string, error) {
	body := request{Model: model, Messages: messages, Stream: false}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("model provider returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("model returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
