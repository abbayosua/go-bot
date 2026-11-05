package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckNamesFailure(t *testing.T) {
	ctx := context.Background()
	bad := New("http://127.0.0.1:1", "key")
	got := Check(ctx, bad, "model")
	if got.OK || got.Reason == "" {
		t.Fatalf("unreachable check: %+v", got)
	}
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"no"}}`)
	}))
	defer denied.Close()
	got = Check(ctx, New(denied.URL, "key"), "model")
	if got.OK || got.Reason != "unauthorized" {
		t.Fatalf("denied check: %+v", got)
	}
	silent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"content":"hi"},"finish_reason":"stop","index":0}]}`)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer silent.Close()
	got = Check(ctx, New(silent.URL, "key"), "model")
	if got.OK || got.Reason != "no_tool_call" {
		t.Fatalf("silent check: %+v", got)
	}
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"provider_check_ping","arguments":"{}"}}]},"finish_reason":"tool_calls","index":0}]}`)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer good.Close()
	got = Check(ctx, New(good.URL, "key"), "model")
	if !got.OK || !got.Stream || !got.Tools {
		t.Fatalf("good check: %+v", got)
	}
}
