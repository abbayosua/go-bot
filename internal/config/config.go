package config

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// Approval modes, most automatic last.
const (
	ApprovalAsk   = "ask"
	ApprovalAuto  = "auto"
	ApprovalNever = "never"
)

type Provider struct {
	BaseURL string
	APIKey  string
	Model   string
	// JudgeModel runs the auto approval check. Empty means auto falls back to asking.
	JudgeModel string
	// FallbackModels are tried in order when the primary chat model fails.
	FallbackModels string
	// CheapModel handles summaries and judges; empty means use the chat model.
	CheapModel string
}

type Config struct {
	Addr     string
	DataDir  string
	Token    string
	Approval string
	Provider Provider
}

// Load reads flags over the environment so the same binary runs as a CLI or a
// service without a config file.
func Load(args []string) (*Config, error) {
	fs := flag.NewFlagSet("go-bot", flag.ContinueOnError)
	addr := fs.String("addr", envOr("GOBOT_ADDR", "127.0.0.1:7417"), "address to listen on")
	data := fs.String("data", envOr("GOBOT_DATA", defaultDataDir()), "directory for the database and bot workspaces")
	token := fs.String("token", os.Getenv("GOBOT_TOKEN"), "bearer token; required when the address is not loopback")
	approval := fs.String("approval", envOr("GOBOT_APPROVAL", ApprovalAsk), "default approval mode: ask, auto or never")
	baseURL := fs.String("base-url", envOr("GOBOT_BASE_URL", "https://api.openai.com/v1"), "OpenAI-compatible API base URL")
	apiKey := fs.String("api-key", os.Getenv("GOBOT_API_KEY"), "model provider API key")
	model := fs.String("model", envOr("GOBOT_MODEL", "gpt-4.1-mini"), "default model")
	judge := fs.String("judge-model", os.Getenv("GOBOT_JUDGE_MODEL"), "model used by the auto approval check")
	fallbacks := fs.String("fallback-models", os.Getenv("GOBOT_FALLBACK_MODELS"), "comma-separated chat fallback models tried in order")
	cheap := fs.String("cheap-model", os.Getenv("GOBOT_CHEAP_MODEL"), "cheaper model for summaries and judges")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg := &Config{
		Addr:     *addr,
		DataDir:  *data,
		Token:    *token,
		Approval: strings.ToLower(*approval),
		Provider: Provider{
			BaseURL:        strings.TrimSuffix(*baseURL, "/"),
			APIKey:         *apiKey,
			Model:          *model,
			JudgeModel:     *judge,
			FallbackModels: *fallbacks,
			CheapModel:     *cheap,
		},
	}
	switch cfg.Approval {
	case ApprovalAsk, ApprovalAuto, ApprovalNever:
	default:
		return nil, fmt.Errorf("approval mode must be ask, auto or never, got %q", cfg.Approval)
	}
	if !isLoopback(cfg.Addr) && cfg.Token == "" {
		return nil, errors.New("a non-loopback address needs -token or GOBOT_TOKEN")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return cfg, nil
}

func (c *Config) DBPath() string { return filepath.Join(c.DataDir, "go-bot.db") }

func (c *Config) Workspaces() string { return filepath.Join(c.DataDir, "workspaces") }

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" {
		return true
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".go-bot"
	}
	return filepath.Join(home, ".go-bot")
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
