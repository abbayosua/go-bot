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
	Store      *store.Store
	Start      func(runID string)
	Log        *slog.Logger
	Workspaces string
	now        func() time.Time
	booted     bool
}

func New(st *store.Store, start func(string), log *slog.Logger) *Scheduler {
	return &Scheduler{Store: st, Start: start, Log: log, now: time.Now}
}

// WithWorkspaces sets the workspace root used to resolve routine watch paths.
func (s *Scheduler) WithWorkspaces(root string) *Scheduler {
	s.Workspaces = root
	return s
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
	s.checkWatches(ctx)
}

// checkWatches fires enabled routines whose watched path changed since the
// last tick, recording the new fingerprint as the run prompt context.
func (s *Scheduler) checkWatches(ctx context.Context) {
	if s.Workspaces == "" {
		return
	}
	watched, err := s.Store.WatchedRoutines(ctx)
	if err != nil {
		s.Log.Error("read watched routines", "error", err)
		return
	}
	for _, routine := range watched {
		current := Fingerprint(s.Workspaces, routine.BotID, routine.WatchPath)
		if current == "refused" {
			continue
		}
		if routine.WatchHash == "" {
			// First sighting records the baseline without firing.
			if err := s.Store.SetRoutineWatch(ctx, routine.ID, routine.WatchPath, current); err != nil {
				s.Log.Error("record watch baseline", "routine", routine.ID, "error", err)
			}
			continue
		}
		if current == routine.WatchHash {
			continue
		}
		prompt := routine.Prompt + " [watch:" + routine.WatchPath + "]"
		run, err := s.Store.CreateRun(ctx, routine.BotID, routine.ThreadID, "watch", prompt)
		if err != nil {
			s.Log.Error("create watch run", "routine", routine.ID, "error", err)
			continue
		}
		if err := s.Store.SetRoutineWatch(ctx, routine.ID, routine.WatchPath, current); err != nil {
			s.Log.Error("advance watch", "routine", routine.ID, "error", err)
		}
		if s.Start != nil {
			s.Start(run.ID)
		}
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
