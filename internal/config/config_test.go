package config

import (
	"path/filepath"
	"testing"
)

func TestLoadDefaultsToLoopbackWithoutToken(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load([]string{"-data", dir})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Addr != "127.0.0.1:7417" {
		t.Errorf("addr %q", cfg.Addr)
	}
	if cfg.Approval != ApprovalAsk {
		t.Errorf("approval %q", cfg.Approval)
	}
	if cfg.DBPath() != filepath.Join(dir, "go-bot.db") {
		t.Errorf("db path %q", cfg.DBPath())
	}
}

func TestLoadRequiresTokenOnPublicAddress(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load([]string{"-data", dir, "-addr", "0.0.0.0:7417"}); err == nil {
		t.Fatal("a public address without a token was accepted")
	}
	cfg, err := Load([]string{"-data", dir, "-addr", "0.0.0.0:7417", "-token", "secret"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Token != "secret" {
		t.Errorf("token %q", cfg.Token)
	}
}

func TestLoadRejectsUnknownApprovalMode(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load([]string{"-data", dir, "-approval", "sometimes"}); err == nil {
		t.Fatal("an invalid approval mode was accepted")
	}
	for _, mode := range []string{"ask", "auto", "never"} {
		if _, err := Load([]string{"-data", dir, "-approval", mode}); err != nil {
			t.Errorf("mode %q: %v", mode, err)
		}
	}
}
