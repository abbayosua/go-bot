package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gobot/internal/store"
)

func TestPreviewArtifactServesInlineWithSandboxHeaders(t *testing.T) {
	ts := newTestServer(t, "never", "write_file")

	status, created := ts.do(t, http.MethodPost, "/api/bots", map[string]string{"name": "Designer"})
	if status != http.StatusCreated {
		t.Fatalf("create bot: %d %v", status, created)
	}
	thread := created["thread"].(map[string]any)
	threadID := thread["id"].(string)
	botID := created["bot"].(map[string]any)["id"].(string)
	bot, err := ts.store.GetBot(t.Context(), botID)
	if err != nil {
		t.Fatalf("get bot: %v", err)
	}

	page := "<html><body><h1>Hi</h1></body></html>"
	if _, err := ts.store.AddMessage(t.Context(), store.Message{
		ThreadID: threadID, Role: "tool", ToolName: "write_file",
		Content: "wrote 20 bytes to index.html", CreatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	artifacts, err := ts.store.ListArtifacts(t.Context(), threadID)
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("artifacts: %v %v", artifacts, err)
	}

	ws := filepath.Join(ts.engine.WorkspacesRoot(), "spaces", bot.SpaceID, "bots", bot.ID)
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "index.html"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(ts.URL + "/api/threads/" + threadID + "/preview?path=index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview status: %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("content-type: %q", ct)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); csp == "" {
		t.Error("missing sandbox CSP")
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		t.Errorf("preview must not force download, got %q", cd)
	}
	body := make([]byte, 512)
	n, _ := resp.Body.Read(body)
	if !strings.Contains(string(body[:n]), `<base href="/api/threads/`) {
		t.Errorf("preview HTML missing injected base tag: %q", string(body[:n]))
	}

	resp2, err := http.Get(ts.URL + "/api/threads/" + threadID + "/preview?path=..%2F..%2Fetc%2Fpasswd")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == http.StatusOK {
		t.Error("path traversal served, want an error status")
	}
}
