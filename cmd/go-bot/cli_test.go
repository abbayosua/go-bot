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
	mux.HandleFunc("/api/bots/bot_1/routines", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"rtn_1","prompt":"p"}`))
			return
		}
		w.Write([]byte(`[{"id":"rtn_1","prompt":"p"}]`))
	})
	mux.HandleFunc("/api/routines/rtn_1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Write([]byte(`{"id":"rtn_1","enabled":true}`))
	})
	mux.HandleFunc("/api/skills", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"skl_1","name":"n"}`))
			return
		}
		w.Write([]byte(`[{"id":"skl_1","name":"n"}]`))
	})
	mux.HandleFunc("/api/skills/skl_1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"rul_1","kind":"allow"}`))
			return
		}
		w.Write([]byte(`[{"id":"rul_1","kind":"allow"}]`))
	})
	mux.HandleFunc("/api/rules/rul_1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.Write([]byte(`{"approval":"never"}`))
			return
		}
		w.Write([]byte(`{"approval":"auto"}`))
	})
	mux.HandleFunc("/api/provider/check", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"model":"m"}`))
	})
	mux.HandleFunc("/api/bots/bot_1/plugins", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"plg_1"}`))
			return
		}
		w.Write([]byte(`[{"id":"plg_1"}]`))
	})
	mux.HandleFunc("/api/plugins/plg_1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Write([]byte(`{"enabled":true}`))
	})
	mux.HandleFunc("/api/mcp/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"mcp_1"}`))
			return
		}
		w.Write([]byte(`[{"id":"mcp_1"}]`))
	})
	mux.HandleFunc("/api/mcp/servers/mcp_1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
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

func TestBatch3UsageErrors(t *testing.T) {
	cases := []struct {
		name string
		fn   func([]string) error
		args []string
	}{
		{"cron-no-sub", runCron, []string{}},
		{"cron-add-no-cron", runCron, []string{"add", "prompt-only"}},
		{"cron-add-no-prompt", runCron, []string{"add", "--cron", "0 9 * * *"}},
		{"cron-edit-no-id", runCron, []string{"edit"}},
		{"cron-edit-no-fields", runCron, []string{"edit", "rtn_1"}},
		{"cron-pause-no-id", runCron, []string{"pause"}},
		{"cron-run-no-id", runCron, []string{"run"}},
		{"cron-run-unknown", runCron, []string{"run", "nope", "--bot", "bot_1"}},
		{"cron-del-no-id", runCron, []string{"del"}},
		{"cron-bad-sub", runCron, []string{"nope"}},
		{"skills-no-sub", runSkills, []string{}},
		{"skills-add-no-body", runSkills, []string{"add", "--name", "n"}},
		{"skills-rm-no-id", runSkills, []string{"rm"}},
		{"skills-bad-sub", runSkills, []string{"nope"}},
		{"rules-no-sub", runRules, []string{}},
		{"rules-add-bad-kind", runRules, []string{"add", "--kind", "maybe"}},
		{"rules-rm-no-id", runRules, []string{"rm"}},
		{"rules-bad-sub", runRules, []string{"nope"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(tc.args); err == nil {
				t.Fatalf("expected usage error, got nil")
			}
		})
	}
}

func TestBatch3LiveStub(t *testing.T) {
	srv := stubStateServer(t)
	defer srv.Close()
	withServer(t, srv.URL)

	if err := runCron([]string{"list", "--bot", "bot_1"}); err != nil {
		t.Fatalf("cron list: %v", err)
	}
	if err := runCron([]string{"add", "--cron", "0 9 * * *", "--bot", "bot_1", "do it"}); err != nil {
		t.Fatalf("cron add: %v", err)
	}
	if err := runCron([]string{"edit", "rtn_1", "--prompt", "new"}); err != nil {
		t.Fatalf("cron edit: %v", err)
	}
	if err := runCron([]string{"pause", "rtn_1"}); err != nil {
		t.Fatalf("cron pause: %v", err)
	}
	if err := runCron([]string{"resume", "rtn_1"}); err != nil {
		t.Fatalf("cron resume: %v", err)
	}
	if err := runCron([]string{"del", "rtn_1"}); err != nil {
		t.Fatalf("cron del: %v", err)
	}
	if err := runSkills([]string{"list"}); err != nil {
		t.Fatalf("skills list: %v", err)
	}
	if err := runSkills([]string{"add", "--name", "n", "--body", "b"}); err != nil {
		t.Fatalf("skills add: %v", err)
	}
	if err := runSkills([]string{"rm", "skl_1"}); err != nil {
		t.Fatalf("skills rm: %v", err)
	}
	if err := runRules([]string{"list"}); err != nil {
		t.Fatalf("rules list: %v", err)
	}
	if err := runRules([]string{"add", "--kind", "allow"}); err != nil {
		t.Fatalf("rules add: %v", err)
	}
	if err := runRules([]string{"rm", "rul_1"}); err != nil {
		t.Fatalf("rules rm: %v", err)
	}
}

func TestBatch4UsageErrors(t *testing.T) {
	cases := []struct {
		name string
		fn   func([]string) error
		args []string
	}{
		{"config-no-sub", runConfig, []string{}},
		{"config-get-bad-key", runConfig, []string{"get", "nope"}},
		{"config-set-short", runConfig, []string{"set", "approval"}},
		{"config-set-bad-key", runConfig, []string{"set", "nope", "x"}},
		{"config-bad-sub", runConfig, []string{"nope"}},
		{"plugins-no-sub", runPlugins, []string{}},
		{"plugins-install-no-path", runPlugins, []string{"install", "--bot", "bot_1"}},
		{"plugins-enable-no-id", runPlugins, []string{"enable"}},
		{"plugins-rm-no-id", runPlugins, []string{"rm"}},
		{"plugins-bad-sub", runPlugins, []string{"nope"}},
		{"mcp-no-sub", runMCP, []string{}},
		{"mcp-add-no-flags", runMCP, []string{"add"}},
		{"mcp-rm-no-id", runMCP, []string{"rm"}},
		{"mcp-bad-sub", runMCP, []string{"nope"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(tc.args); err == nil {
				t.Fatalf("expected usage error, got nil")
			}
		})
	}
}

func TestBatch4LiveStub(t *testing.T) {
	srv := stubStateServer(t)
	defer srv.Close()
	withServer(t, srv.URL)

	if err := runConfig([]string{"get"}); err != nil {
		t.Fatalf("config get: %v", err)
	}
	if err := runConfig([]string{"get", "approval"}); err != nil {
		t.Fatalf("config get approval: %v", err)
	}
	if err := runConfig([]string{"set", "approval", "never"}); err != nil {
		t.Fatalf("config set: %v", err)
	}
	if err := runModel([]string{}); err != nil {
		t.Fatalf("model: %v", err)
	}
	if err := runModel([]string{"--check"}); err != nil {
		t.Fatalf("model check: %v", err)
	}
	if err := runStatus([]string{}); err != nil {
		t.Fatalf("status: %v", err)
	}
	if err := runDoctor([]string{}); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if err := runPlugins([]string{"list", "--bot", "bot_1"}); err != nil {
		t.Fatalf("plugins list: %v", err)
	}
	if err := runPlugins([]string{"install", "--bot", "bot_1", "p"}); err != nil {
		t.Fatalf("plugins install: %v", err)
	}
	if err := runPlugins([]string{"enable", "plg_1"}); err != nil {
		t.Fatalf("plugins enable: %v", err)
	}
	if err := runPlugins([]string{"disable", "plg_1"}); err != nil {
		t.Fatalf("plugins disable: %v", err)
	}
	if err := runPlugins([]string{"rm", "plg_1"}); err != nil {
		t.Fatalf("plugins rm: %v", err)
	}
	if err := runMCP([]string{"list"}); err != nil {
		t.Fatalf("mcp list: %v", err)
	}
	if err := runMCP([]string{"add", "--name", "n", "--url", "http://x"}); err != nil {
		t.Fatalf("mcp add: %v", err)
	}
	if err := runMCP([]string{"rm", "mcp_1"}); err != nil {
		t.Fatalf("mcp rm: %v", err)
	}
}

func TestUnknownCommand(t *testing.T) {
	if err := runCLI("nope", nil); err == nil {
		t.Fatalf("expected unknown command error")
	}
	_ = json.RawMessage(`{}`)
	_ = os.Getenv("GOBOT_URL")
}
