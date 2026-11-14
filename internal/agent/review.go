package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gobot/internal/bus"
	"gobot/internal/llm"
	"gobot/internal/store"
)

// reviewGate decides whether a finished turn deserves a model review.
// Hermes reviews every turn; Go-Bot only reviews on cheap local signals so
// 80-90% of turns cost zero extra model calls.
type reviewSignal struct {
	fired  bool
	reason string
}

// gateReview scans the finished turn for learning signals without any model
// call: explicit remember requests, corrections, tool failures, denied
// approvals. Mirrors the Hermes _SKILL/_MEMORY review triggers.
func gateReview(userText, assistantText string, toolFails, denied int) reviewSignal {
	u := strings.ToLower(userText)
	a := strings.ToLower(assistantText)
	explicit := []string{
		"ingat ini", "remember this", "jangan lupa",
		"mulai sekarang", "from now on",
		"jangan begitu", "jangan seperti", "don't do that", "stop doing",
		"terlalu panjang", "terlalu verbose", "too verbose", "too long",
		"kepanjangan", "singkat saja", "just give me the answer",
	}
	for _, s := range explicit {
		if strings.Contains(u, s) || strings.Contains(a, s) {
			return reviewSignal{true, "explicit:" + s}
		}
	}
	if toolFails >= 2 {
		return reviewSignal{true, "tools-failed"}
	}
	if denied > 0 {
		return reviewSignal{true, "approval-denied"}
	}
	corrections := []string{"salah", "bukan begitu", "wrong", "correction", "maksud saya", "i meant"}
	for _, s := range corrections {
		if strings.Contains(u, s) {
			return reviewSignal{true, "correction:" + s}
		}
	}
	return reviewSignal{}
}

// reviewOutcome is the single JSON object the review model must return.
type reviewOutcome struct {
	Memory string `json:"memory"`
	Skill  string `json:"skill"`
}

const reviewSystemPrompt = `You review one finished assistant turn. Reply with ONE JSON object only: {"memory":"...","skill":"..."}.
- "memory": a durable fact about the user (preference, persona, instruction). Empty string if none.
- "skill": a reusable technique, fix, or workflow correction worth saving. Empty string if none.
Be conservative: empty strings are fine. Never invent names or secrets.`

// lastExchange returns the latest user text and the latest assistant text in
// a thread (DB read, no model call).
func (e *Engine) lastExchange(ctx context.Context, threadID string) (user, assistant string) {
	msgs, err := e.Store.ListMessages(ctx, threadID)
	if err != nil {
		return "", ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if assistant == "" && m.Role == "assistant" && strings.TrimSpace(m.Content) != "" {
			assistant = m.Content
		}
		if user == "" && m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			user = m.Content
		}
		if user != "" && assistant != "" {
			break
		}
	}
	return user, assistant
}

// turnSignals counts cheap local failure signals for one run: tool messages
// recording errors and approvals the user denied.
func (e *Engine) turnSignals(ctx context.Context, runID, threadID string) (toolFails, denied int) {
	msgs, err := e.Store.ListMessages(ctx, threadID)
	if err != nil {
		return 0, 0
	}
	for _, m := range msgs {
		if m.ToolCallID == "" {
			continue
		}
		c := strings.ToLower(m.Content)
		if strings.Contains(c, "error") || strings.Contains(c, "failed") || strings.Contains(c, "denied") {
			toolFails++
		}
	}
	apvs, err := e.Store.ListApprovals(ctx, threadID)
	if err != nil {
		return toolFails, 0
	}
	for _, a := range apvs {
		if a.Status == store.ApprovalDeny {
			denied++
		}
	}
	return toolFails, denied
}

// maybeReview runs the gated Hermes-style self-review after a finished turn.
// Memory writes land directly (user's own facts). Skill writes go through
// the approval queue so nothing teaches the bot without consent.
func (e *Engine) maybeReview(run store.Run, bot store.Bot, thread store.Thread) {
	userText, assistantText := e.lastExchange(context.Background(), thread.ID)
	toolFails, denied := e.turnSignals(context.Background(), run.ID, thread.ID)
	sig := gateReview(userText, assistantText, toolFails, denied)
	if !sig.fired || e.Client == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		model := bot.Model
		if strings.TrimSpace(model) == "" {
			model = e.Provider.Model
		}
		transcript := "USER: " + truncateReview(userText, 1500) + "\nASSISTANT: " + truncateReview(assistantText, 2500)
		text, err := e.Client.Complete(ctx, model, []llm.Message{
			{Role: "system", Content: reviewSystemPrompt},
			{Role: "user", Content: transcript},
		})
		if err != nil {
			e.Log.Error("self-review failed", "run", run.ID, "error", err)
			return
		}
		var out reviewOutcome
		if err := json.Unmarshal([]byte(extractReviewJSON(text)), &out); err != nil {
			e.Log.Error("self-review parse failed", "run", run.ID)
			return
		}
		if fact := strings.TrimSpace(out.Memory); fact != "" {
			if _, err := e.Store.AddMemory(ctx, bot.ID, fact); err != nil {
				e.Log.Error("self-review memory save failed", "run", run.ID, "error", err)
			} else {
				e.Log.Info("self-review saved memory", "run", run.ID, "signal", sig.reason)
			}
		}
		if skill := strings.TrimSpace(out.Skill); skill != "" {
			callID := "review-" + run.ID
			stored, err := e.Store.CreateApproval(ctx, run.ID, thread.ID, callID, "remember-skill", skill)
			if err != nil {
				e.Log.Error("self-review skill queue failed", "run", run.ID, "error", err)
			} else {
				if full, gerr := e.Store.GetApproval(ctx, stored.ID); gerr == nil {
					stored = full
				}
				e.publish(thread.ID, bus.Event{Type: "approval", Data: stored})
				e.Log.Info("self-review queued skill", "run", run.ID, "signal", sig.reason)
			}
		}
	}()
}

func truncateReview(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// extractReviewJSON tolerates fences around the single JSON object.
func extractReviewJSON(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			return s[i : j+1]
		}
	}
	return s
}
