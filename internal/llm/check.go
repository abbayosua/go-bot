package llm

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// CheckResult names what a provider probe found.
type CheckResult struct {
	OK         bool   `json:"ok"`
	Stream     bool   `json:"stream"`
	Tools      bool   `json:"tools"`
	Reason     string `json:"reason,omitempty"`
	Detail     string `json:"detail,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

// Check verifies a provider can stream and emit a tool call, failing with a
// named reason otherwise. It uses the probe tool only: nothing is executed.
func Check(ctx context.Context, c *Client, model string) CheckResult {
	start := time.Now()
	result := CheckResult{}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	probe := []Tool{{
		Type:     "function",
		Function: FunctionDef{Name: "provider_check_ping", Description: "Reply through this tool to prove tool calling works.", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)},
	}}
	res, err := c.Stream(ctx, model, []Message{
		{Role: "system", Content: "You must call provider_check_ping and nothing else."},
		{Role: "user", Content: "Call provider_check_ping now."},
	}, probe, nil)
	result.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		result.Reason = nameFailure(err)
		result.Detail = err.Error()
		return result
	}
	result.Stream = true
	for _, call := range res.ToolCalls {
		if call.Function.Name == "provider_check_ping" {
			result.Tools = true
		}
	}
	if !result.Tools {
		result.Reason = "no_tool_call"
		result.Detail = "provider streamed but never called provider_check_ping"
		return result
	}
	result.OK = true
	return result
}

func nameFailure(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no stream"):
		return "no_stream"
	case strings.Contains(msg, "returned 401") || strings.Contains(msg, "returned 403"):
		return "unauthorized"
	case strings.Contains(msg, "returned 404"):
		return "not_found"
	case strings.Contains(msg, "returned 429"):
		return "rate_limited"
	case strings.Contains(strings.ToLower(msg), "timeout") || strings.Contains(strings.ToLower(msg), "deadline"):
		return "timeout"
	default:
		return "request_failed"
	}
}
