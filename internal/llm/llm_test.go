package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sseServer(t *testing.T, payloads ...string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, payload := range payloads {
			fmt.Fprintf(w, "data: %s\n\n", payload)
		}
		io.WriteString(w, "data: [DONE]\n\n")
	}))
}

func TestStreamAssemblesTextAndToolCalls(t *testing.T) {
	server := sseServer(t,
		`{"choices":[{"delta":{"content":"Hel"}}]}`,
		`{"choices":[{"delta":{"content":"lo"}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"shell","arguments":"{\"comm"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"and\":\"ls\"}"}}]},"finish_reason":"tool_calls"}]}`,
	)
	defer server.Close()

	var streamed strings.Builder
	result, err := New(server.URL, "key").Stream(context.Background(), "m", []Message{{Role: "user", Content: "hi"}}, nil, func(text string) {
		streamed.WriteString(text)
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if result.Content != "Hello" {
		t.Errorf("content %q", result.Content)
	}
	if streamed.String() != "Hello" {
		t.Errorf("streamed %q", streamed.String())
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls", len(result.ToolCalls))
	}
	call := result.ToolCalls[0]
	if call.ID != "call_1" || call.Function.Name != "shell" {
		t.Errorf("call %+v", call)
	}
	if call.Function.Arguments != `{"command":"ls"}` {
		t.Errorf("arguments %q", call.Function.Arguments)
	}
	if result.FinishReason != "tool_calls" {
		t.Errorf("finish reason %q", result.FinishReason)
	}
}

func TestStreamSendsAuthAndModel(t *testing.T) {
	var gotAuth, gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotModel = string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	if _, err := New(server.URL+"/", "secret").Stream(context.Background(), "gpt-test", nil, nil, nil); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("authorization %q", gotAuth)
	}
	if !strings.Contains(gotModel, `"model":"gpt-test"`) || !strings.Contains(gotModel, `"stream":true`) {
		t.Errorf("body %s", gotModel)
	}
}

func TestStreamReportsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"bad key"}}`)
	}))
	defer server.Close()

	_, err := New(server.URL, "").Stream(context.Background(), "m", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("error %v", err)
	}
}

func TestStreamRejectsNonStreamingResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[]}`)
	}))
	defer server.Close()

	if _, err := New(server.URL, "").Stream(context.Background(), "m", nil, nil, nil); err == nil {
		t.Fatal("a non-streaming response was accepted")
	}
}

func TestCompleteReturnsText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"ALLOW"}}]}`)
	}))
	defer server.Close()

	answer, err := New(server.URL, "").Complete(context.Background(), "judge", nil)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if answer != "ALLOW" {
		t.Errorf("answer %q", answer)
	}
}
