package schedule

import (
	"context"
	"log/slog"
	"time"

	"gobot/internal/store"
)

const tickInterval = 15 * time.Second

// Scheduler turns a due routine into a run. There is no queue: a due tick
// starts the work immediately in this process.
type Scheduler struct {
	Store  *store.Store
	Start  func(runID string)
	Log    *slog.Logger
	now    func() time.Time
	booted bool
}

func New(st *store.Store, start func(string), log *slog.Logger) *Scheduler {
	return &Scheduler{Store: st, Start: start, Log: log, now: time.Now}
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	now := s.now()
	due, err := s.Store.DueRoutines(ctx, now.Unix())
	if err != nil {
		s.Log.Error("read due routines", "error", err)
		return
	}
	for _, routine := range due {
		s.fire(ctx, routine, now)
	}
}

func (s *Scheduler) fire(ctx context.Context, routine store.Routine, now time.Time) {
	run, err := s.Store.CreateRun(ctx, routine.BotID, routine.ThreadID, "routine", routine.Prompt)
	if err != nil {
		s.Log.Error("create routine run", "routine", routine.ID, "error", err)
		return
	}
	next := nextAfter(routine.Cron, now)
	if err := s.Store.MarkRoutineFired(ctx, routine.ID, now.Unix(), next); err != nil {
		s.Log.Error("advance routine", "routine", routine.ID, "error", err)
	}
	s.Log.Info("routine fired", "routine", routine.Name, "run", run.ID, "next", next)
	if s.Start != nil {
		s.Start(run.ID)
	}
}

func nextAfter(expr string, now time.Time) int64 {
	parsed, err := Parse(expr)
	if err != nil {
		return 0
	}
	next, err := parsed.Next(now)
	if err != nil {
		return 0
	}
	return next.Unix()
}
