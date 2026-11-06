// Package mcp speaks the Model Context Protocol's JSON-RPC surface just far
// enough to list and call tools: initialize, tools/list, tools/call, over
// stdio (spawned command) or streamable HTTP (POST). Anything
// server-specific stays in the server, not in this client.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Transport is how a server is reached.
type Transport string

const (
	TransportStdio Transport = "stdio"
	TransportHTTP  Transport = "http"
)

// Server describes one MCP server.
type Server struct {
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (s Server) transport() Transport {
	if strings.TrimSpace(s.URL) != "" {
		return TransportHTTP
	}
	return TransportStdio
}

// Tool is one MCP tool advertisement.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
}

// Client talks to one server.
type Client struct {
	server Server
	http   *http.Client
	mu     sync.Mutex
	nextID int64
}

func NewClient(server Server) *Client {
	return &Client{server: server, http: &http.Client{Timeout: 60 * time.Second}}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

func (c *Client) next() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	return c.nextID
}

// ListTools runs initialize then tools/list.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	if _, err := c.initialize(ctx); err != nil {
		return nil, err
	}
	raw, err := c.call(ctx, "tools/list", map[string]any{"cursor": ""})
	if err != nil {
		return nil, err
	}
	var list struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("decode tools/list: %w", err)
	}
	out := make([]Tool, 0, len(list.Tools))
	for _, tool := range list.Tools {
		out = append(out, Tool{Name: tool.Name, Description: tool.Description, Schema: tool.InputSchema})
	}
	return out, nil
}

// CallTool runs tools/call and returns the text content.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	raw, err := c.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return "", err
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode tools/call: %w", err)
	}
	var texts []string
	for _, content := range result.Content {
		if content.Type == "text" {
			texts = append(texts, content.Text)
		}
	}
	text := strings.Join(texts, "\n")
	if result.IsError {
		return text, errors.New(text)
	}
	if text == "" {
		return string(raw), nil
	}
	return text, nil
}

// Health runs initialize only: a cheap liveness probe for the server list.
func (c *Client) Health(ctx context.Context) (string, error) {
	init, err := c.initialize(ctx)
	if err != nil {
		return "", err
	}
	if init.ServerInfo.Name == "" {
		return "ok", nil
	}
	return init.ServerInfo.Name + " " + init.ServerInfo.Version, nil
}

func (c *Client) initialize(ctx context.Context) (initializeResult, error) {
	var out initializeResult
	raw, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "go-bot", "version": "1"},
	})
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decode initialize: %w", err)
	}
	return out, nil
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	req := rpcRequest{JSONRPC: "2.0", ID: c.next(), Method: method, Params: params}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	switch c.server.transport() {
	case TransportHTTP:
		return c.callHTTP(ctx, payload)
	default:
		return c.callStdio(ctx, payload)
	}
}

func (c *Client) callHTTP(ctx context.Context, payload []byte) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.server.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for key, value := range c.server.Headers {
		req.Header.Set(key, value)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mcp http status %d", resp.StatusCode)
	}
	return decodeResponse(detachSSE(body))
}

func (c *Client) callStdio(ctx context.Context, payload []byte) (json.RawMessage, error) {
	parts := strings.Fields(c.server.Command)
	if len(parts) == 0 {
		return nil, errors.New("stdio server needs a command")
	}
	// #nosec G204 -- the command comes from the operator's own server list.
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Stdin = bytes.NewReader(append(payload, '\n'))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("stdio server: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "{") {
			return decodeResponse([]byte(trimmed))
		}
	}
	return nil, errors.New("stdio server sent no JSON-RPC reply")
}

func detachSSE(body []byte) []byte {
	text := strings.TrimSpace(string(body))
	if !strings.Contains(text, "data:") {
		return body
	}
	for _, line := range strings.Split(text, "\n") {
		if trimmed, ok := strings.CutPrefix(strings.TrimSpace(line), "data:"); ok {
			if trimmed = strings.TrimSpace(trimmed); strings.HasPrefix(trimmed, "{") {
				return []byte(trimmed)
			}
		}
	}
	return body
}

func decodeResponse(body []byte) (json.RawMessage, error) {
	var resp rpcResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode rpc: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}
