package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// stubStateServer returns a fake go-bot server covering Batch 1 routes.
func stubStateServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","version":"test"}`))
	})
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"approval":"auto","bots":[{"id":"bot_1","name":"B1","threads":[{"id":"thr_1","title":"Chat"}]}],"spaces":[]}`))
	})
	mux.HandleFunc("/api/threads/thr_1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"thr_1","messages":[{"id":"m1","role":"assistant","content":"hi"}]}`))
	})
	mux.HandleFunc("/api/threads/thr_1/messages", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"message":{"id":"m2"},"run":{"id":"run_1"}}`))
	})
	mux.HandleFunc("/api/bots", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"bot":{"id":"bot_9"},"thread":{"id":"thr_9"}}`))
	})
	mux.HandleFunc("/api/bots/bot_1/threads", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"thr_new"}`))
			return
		}
		w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/api/bots/bot_1/threads/thr_1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/approvals/apv_1", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"apv_1","decision":"allow"}`))
	})
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":"m1","content":"hit"}]`))
	})
	mux.HandleFunc("/api/audit", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") == "csv" {
			w.Header().Set("Content-Type", "text/csv")
			w.Write([]byte("id,action\n"))
			return
		}
		w.Write([]byte(`[{"id":"aud_1","action":"x"}]`))
	})
	mux.HandleFunc("/api/bots/bot_1/memories", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":"mem_1","content":"fact"}]`))
	})
	mux.HandleFunc("/api/memories/mem_1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/bots/bot_1/scratchpad", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(`["n1"]`))
		case http.MethodPost:
			w.Write([]byte(`{"status":"noted"}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
	mux.HandleFunc("/api/bots/bot_1/board", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"brd_1"}`))
			return
		}
		w.Write([]byte(`[{"id":"brd_1","title":"t"}]`))
	})
	mux.HandleFunc("/api/board/brd_1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Write([]byte(`{"id":"brd_1","status":"doing"}`))
	})
	return httptest.NewServer(mux)
}

func withServer(t *testing.T, url string) {
	t.Helper()
	t.Setenv("GOBOT_URL", url)
	t.Setenv("GOBOT_TOKEN", "")
}

func TestBatch1UsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"chat-no-msg", []string{}},
		{"bots-no-sub", []string{}},
		{"bots-create-no-name", []string{"create"}},
		{"bots-rm-no-id", []string{"rm"}},
		{"threads-no-sub", []string{}},
		{"threads-rm-short", []string{"rm", "only-one"}},
		{"threads-open-no-id", []string{"open"}},
		{"approve-no-args", []string{}},
		{"approve-bad-decision", []string{"apv_1", "maybe"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			switch {
			case len(tc.name) >= 4 && tc.name[:4] == "chat":
				err = runChat(tc.args)
			case len(tc.name) >= 4 && tc.name[:4] == "bots":
				err = runBots(tc.args)
			case len(tc.name) >= 7 && tc.name[:7] == "threads":
				err = runThreads(tc.args)
			default:
				err = runApprove(tc.args)
			}
			if err == nil {
				t.Fatalf("expected usage error, got nil")
			}
		})
	}
}

func TestBatch1LiveStub(t *testing.T) {
	srv := stubStateServer(t)
	defer srv.Close()
	withServer(t, srv.URL)

	if err := runState(nil); err != nil {
		t.Fatalf("state: %v", err)
	}
	if err := runBots([]string{"list"}); err != nil {
		t.Fatalf("bots list: %v", err)
	}
	if err := runBots([]string{"create", "--name", "T", "--url", srv.URL}); err != nil {
		t.Fatalf("bots create: %v", err)
	}
	if err := runThreads([]string{"list"}); err != nil {
		t.Fatalf("threads list: %v", err)
	}
	if err := runThreads([]string{"new", "--bot", "bot_1"}); err != nil {
		t.Fatalf("threads new: %v", err)
	}
	if err := runThreads([]string{"rm", "bot_1", "thr_1"}); err != nil {
		t.Fatalf("threads rm: %v", err)
	}
	if err := runThreads([]string{"open", "thr_1"}); err != nil {
		t.Fatalf("threads open: %v", err)
	}
	if err := runApprove([]string{"apv_1", "allow"}); err != nil {
		t.Fatalf("approve: %v", err)
	}
}

func TestResolveThread(t *testing.T) {
	srv := stubStateServer(t)
	defer srv.Close()
	withServer(t, srv.URL)
	c := newCLIClient("")
	before, id, err := resolveThread(c, "", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if id != "thr_1" || before != 1 {
		t.Fatalf("got id=%q before=%d", id, before)
	}
	if _, _, err := resolveThread(c, "nope", ""); err == nil {
		t.Fatalf("expected bot-not-found error")
	}
}

func TestBatch2UsageErrors(t *testing.T) {
	cases := []struct {
		name string
		fn   func([]string) error
		args []string
	}{
		{"search-no-q", runSearch, []string{}},
		{"memory-no-sub", runMemory, []string{}},
		{"memory-forget-no-id", runMemory, []string{"forget"}},
		{"memory-bad-sub", runMemory, []string{"nope"}},
		{"notes-no-sub", runNotes, []string{}},
		{"notes-add-no-text", runNotes, []string{"add"}},
		{"notes-add-flag-only", runNotes, []string{"add", "--bot", "bot_1"}},
		{"notes-bad-sub", runNotes, []string{"nope"}},
		{"board-no-sub", runBoard, []string{}},
		{"board-add-no-title", runBoard, []string{"add"}},
		{"board-add-flag-only", runBoard, []string{"add", "--bot", "bot_1"}},
		{"board-move-short", runBoard, []string{"move", "only-one"}},
		{"board-del-no-id", runBoard, []string{"del"}},
		{"board-bad-sub", runBoard, []string{"nope"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(tc.args); err == nil {
				t.Fatalf("expected usage error, got nil")
			}
		})
	}
}

func TestBatch2LiveStub(t *testing.T) {
	srv := stubStateServer(t)
	defer srv.Close()
	withServer(t, srv.URL)

	if err := runSearch([]string{"hello"}); err != nil {
		t.Fatalf("search: %v", err)
	}
	if err := runAudit([]string{"--limit", "5"}); err != nil {
		t.Fatalf("audit json: %v", err)
	}
	if err := runAudit([]string{"--csv"}); err != nil {
		t.Fatalf("audit csv: %v", err)
	}
	if err := runMemory([]string{"list", "--bot", "bot_1"}); err != nil {
		t.Fatalf("memory list: %v", err)
	}
	if err := runMemory([]string{"forget", "mem_1"}); err != nil {
		t.Fatalf("memory forget: %v", err)
	}
	if err := runNotes([]string{"list", "--bot", "bot_1"}); err != nil {
		t.Fatalf("notes list: %v", err)
	}
	if err := runNotes([]string{"add", "hello", "--bot", "bot_1"}); err != nil {
		t.Fatalf("notes add: %v", err)
	}
	if err := runNotes([]string{"clear", "--bot", "bot_1"}); err != nil {
		t.Fatalf("notes clear: %v", err)
	}
	if err := runBoard([]string{"list", "--bot", "bot_1"}); err != nil {
		t.Fatalf("board list: %v", err)
	}
	if err := runBoard([]string{"add", "card", "--bot", "bot_1"}); err != nil {
		t.Fatalf("board add: %v", err)
	}
	if err := runBoard([]string{"move", "brd_1", "doing"}); err != nil {
		t.Fatalf("board move: %v", err)
	}
	if err := runBoard([]string{"del", "brd_1"}); err != nil {
		t.Fatalf("board del: %v", err)
	}
}

func TestUnknownCommand(t *testing.T) {
	if err := runCLI("nope", nil); err == nil {
		t.Fatalf("expected unknown command error")
	}
	_ = json.RawMessage(`{}`)
	_ = os.Getenv("GOBOT_URL")
}
