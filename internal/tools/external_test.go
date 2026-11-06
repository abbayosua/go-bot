package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gobot/internal/store"
)

func TestExternalDelegateRoundTrip(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bot, err := st.CreateBot(ctx, "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	var gotTask, gotToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Task  string `json:"task"`
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotTask, gotToken = body.Task, body.Token
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	agent, err := st.AddExternalAgent(ctx, "Out", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	tool := &externalTool{store: st}
	raw, _ := json.Marshal(map[string]string{"agent": agent.Name, "task": "research x"})
	done := make(chan string, 1)
	go func() {
		out, err := tool.RunFor(WithExternalRun(ctx, "run_9"), bot.ID, raw)
		if err != nil {
			done <- "ERR:" + err.Error()
			return
		}
		done <- out
	}()
	for i := 0; i < 100 && gotToken == ""; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if gotTask != "research x" || gotToken == "" {
		t.Fatalf("dispatch task=%q token=%q", gotTask, gotToken)
	}
	if _, err := st.CompleteExternalTask(ctx, gotToken, "it is done"); err != nil {
		t.Fatal(err)
	}
	select {
	case out := <-done:
		if !strings.Contains(out, "it is done") || !strings.Contains(out, "Out") {
			t.Fatalf("answer %q", out)
		}
	case <-time.After(70 * time.Second):
		t.Fatal("tool never returned")
	}
}
