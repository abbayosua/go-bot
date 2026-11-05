package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gobot/internal/agent"
	"gobot/internal/bus"
	"gobot/internal/config"
	"gobot/internal/httpapi"
	"gobot/internal/llm"
	"gobot/internal/schedule"
	"gobot/internal/store"
	"gobot/web"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("go-bot stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return err
	}
	st, err := store.Open(cfg.DBPath())
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := seed(ctx, st); err != nil {
		return err
	}

	events := bus.New()
	client := llm.New(cfg.Provider.BaseURL, cfg.Provider.APIKey)
	engine := agent.NewEngine(st, client, events, cfg.Provider, cfg.Workspaces(), cfg.Approval, log)
	scheduler := schedule.New(st, engine.Start, log).WithWorkspaces(cfg.Workspaces())
	go scheduler.Run(ctx)

	assets, err := fs.Sub(web.Files, ".")
	if err != nil {
		return err
	}
	server := &httpapi.Server{
		Store:    st,
		Engine:   engine,
		Bus:      events,
		Provider: cfg.Provider,
		Token:    cfg.Token,
		Log:      log,
		Assets:   assets,
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()

	log.Info("go-bot listening", "addr", cfg.Addr, "data", cfg.DataDir, "approval", cfg.Approval, "model", cfg.Provider.Model)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// seed gives a fresh install one bot so the first screen is usable.
func seed(ctx context.Context, st *store.Store) error {
	bots, err := st.ListBots(ctx)
	if err != nil || len(bots) > 0 {
		return err
	}
	bot, err := st.CreateBot(ctx, "Assistant", "You are a direct, capable assistant. Finish the task, then report briefly.", "")
	if err != nil {
		return err
	}
	thread, err := st.CreateThread(ctx, bot.ID, "Chat")
	if err != nil {
		return err
	}
	// First-run onboarding: the empty sidebar becomes a working bot with one
	// guided message instead of a blank thread.
	_, err = st.AddMessage(ctx, store.Message{
		ThreadID: thread.ID,
		Role:     "assistant",
		Content:  "Hi, I'm your assistant. Tell me what to do — for example, ask me to list the workspace files. Type a message below to start.",
	})
	return err
}
