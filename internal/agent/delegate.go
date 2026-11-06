package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gobot/internal/store"
)

// Delegation bounds: a parent run fans out to at most this many children,
// and chains nest at most this deep. Both stop a runaway mesh of bots.
const (
	delegateMaxDepth = 3
	delegateMaxCount = 3
)

// delegate runs a peer bot (or a fresh short-lived subagent) synchronously
// and returns its final answer. Depth comes from the parent run's chain;
// count is tracked per parent run id.
func (e *Engine) delegate(ctx context.Context, parent store.Run, target, task string, fresh bool) (string, error) {
	depth := delegateDepth(ctx)
	if depth >= delegateMaxDepth {
		return "", errors.New("delegation depth exceeded")
	}
	if e.delegateCount(parent.ID) >= delegateMaxCount {
		return "", errors.New("delegation count exceeded for this run")
	}
	var bot store.Bot
	var err error
	if fresh {
		name := strings.TrimSpace(target)
		if name == "" {
			name = "Subagent"
		}
		parent2, _ := e.Store.GetBot(ctx, parent.BotID)
		spaceID := ""
		if parent2.ID != "" {
			spaceID = parent2.SpaceID
		}
		bot, err = e.Store.CreateBotIn(ctx, spaceID, name+" (subagent)", "You are a short-lived subagent. Do the task, report back briefly.", "")
		if err != nil {
			return "", err
		}
	} else {
		bot, err = e.Store.FindBot(ctx, strings.TrimSpace(target))
		if err != nil {
			return "", err
		}
		if bot.ID == parent.BotID {
			return "", errors.New("delegate to another bot, not yourself")
		}
	}
	thread, err := e.Store.CreateThread(ctx, bot.ID, "Delegated: "+truncateTask(task))
	if err != nil {
		return "", err
	}
	if _, err := e.Store.AddMessage(ctx, store.Message{ThreadID: thread.ID, Role: "user", Content: task}); err != nil {
		return "", err
	}
	run, err := e.Store.CreateRun(ctx, bot.ID, thread.ID, "delegate", task)
	if err != nil {
		return "", err
	}
	e.bumpDelegateCount(parent.ID)
	if err := e.executeChild(delegateCtx(ctx), run.ID); err != nil {
		return "", err
	}
	return e.childAnswer(ctx, run)
}

// executeChild runs a delegated run inline with the incremented depth.
func (e *Engine) executeChild(ctx context.Context, runID string) error {
	return e.Execute(ctx, runID)
}

// childAnswer reads the child's final assistant message.
func (e *Engine) childAnswer(ctx context.Context, run store.Run) (string, error) {
	messages, err := e.Store.ListMessages(ctx, run.ThreadID)
	if err != nil {
		return "", err
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && strings.TrimSpace(messages[i].Content) != "" {
			return fmt.Sprintf("From %s: %s", childName(ctx, e.Store, run.BotID), messages[i].Content), nil
		}
	}
	finished, err := e.Store.GetRun(ctx, run.ID)
	if err != nil {
		return "", err
	}
	if finished.Status != store.RunDone {
		return "", fmt.Errorf("delegated run %s", finished.Status)
	}
	return "the subagent finished with no reply", nil
}

func childName(ctx context.Context, st *store.Store, botID string) string {
	if bot, err := st.GetBot(ctx, botID); err == nil {
		return bot.Name
	}
	return "subagent"
}

func truncateTask(task string) string {
	task = strings.TrimSpace(task)
	if len(task) > 60 {
		return task[:60] + "…"
	}
	if task == "" {
		return "task"
	}
	return task
}

// delegate chain bookkeeping: depth rides the context, count rides memory.
type delegateDepthKey struct{}

func delegateCtx(ctx context.Context) context.Context {
	return context.WithValue(ctx, delegateDepthKey{}, delegateDepth(ctx)+1)
}

func delegateDepth(ctx context.Context) int {
	if depth, ok := ctx.Value(delegateDepthKey{}).(int); ok {
		return depth
	}
	return 0
}

func (e *Engine) delegateCount(runID string) int {
	e.countMu.Lock()
	defer e.countMu.Unlock()
	return e.delegateCounts[runID]
}

func (e *Engine) bumpDelegateCount(runID string) {
	e.countMu.Lock()
	defer e.countMu.Unlock()
	e.delegateCounts[runID]++
}
