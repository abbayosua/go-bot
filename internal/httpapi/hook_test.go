package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gobot/internal/agent"
	"gobot/internal/bus"
	"gobot/internal/config"
	"gobot/internal/llm"
	"gobot/internal/store"
)

func signedServer(t *testing.T) (*httptest.Server, *store.Store, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := bus.New()
	engine := agent.NewEngine(st, llm.New("http://127.0.0.1:1", ""), events, config.Provider{Model: "test"}, t.TempDir(), config.ApprovalNever, log)
	server := &Server{Store: st, Engine: engine, Bus: events, Provider: config.Provider{Model: "test"}, Token: "secret-token", Log: log}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return httpServer, st, "secret-token"
}

func sign(token string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestInboundHookFiresRun(t *testing.T) {
	ctx := context.Background()
	httpServer, st, token := signedServer(t)
	bot, err := st.CreateBot(ctx, "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := st.CreateThread(ctx, bot.ID, "Chat")
	if err != nil {
		t.Fatal(err)
	}
	routine, err := st.CreateRoutine(ctx, bot.ID, thread.ID, "R", "report", "0 0 * * *", 0)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"event":"push"}`)
	req, _ := http.NewRequest(http.MethodPost, httpServer.URL+"/api/hooks/"+routine.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Signature", sign(token, body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted || out["runId"] == "" {
		t.Fatalf("hook: %d %v", resp.StatusCode, out)
	}
	run, err := st.GetRun(ctx, out["runId"])
	if err != nil {
		t.Fatal(err)
	}
	if run.Trigger != "webhook" {
		t.Fatalf("trigger %q, want webhook", run.Trigger)
	}
	bad, _ := http.NewRequest(http.MethodPost, httpServer.URL+"/api/hooks/"+routine.ID, bytes.NewReader(body))
	bad.Header.Set("Authorization", "Bearer "+token)
	bad.Header.Set("X-Signature", "wrong")
	badResp, err := http.DefaultClient.Do(bad)
	if err != nil {
		t.Fatal(err)
	}
	badResp.Body.Close()
	if badResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad signature: %d", badResp.StatusCode)
	}
	missing, _ := http.NewRequest(http.MethodPost, httpServer.URL+"/api/hooks/rtn_missing", bytes.NewReader(body))
	missing.Header.Set("Authorization", "Bearer "+token)
	missing.Header.Set("X-Signature", sign(token, body))
	missingResp, err := http.DefaultClient.Do(missing)
	if err != nil {
		t.Fatal(err)
	}
	missingResp.Body.Close()
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing routine: %d", missingResp.StatusCode)
	}
}
