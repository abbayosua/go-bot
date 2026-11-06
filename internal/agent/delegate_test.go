package agent

import (
	"context"
	"strings"
	"testing"
)

func TestDelegateBounds(t *testing.T) {
	h := newHarness(t, "never", nil)
	ctx := context.Background()
	run, err := h.store.CreateRun(ctx, h.bot.ID, h.thread.ID, "user", "go")
	if err != nil {
		t.Fatal(err)
	}
	// Self-delegation refused.
	if _, err := h.engine.delegate(ctx, run, h.bot.Name, "task", false); err == nil {
		t.Fatal("self delegation allowed")
	}
	// Unknown peer refused.
	if _, err := h.engine.delegate(ctx, run, "Ghost", "task", false); err == nil {
		t.Fatal("unknown peer allowed")
	}
	// Count bound: 3 delegations per run, 4th refused.
	for i := 0; i < delegateMaxCount; i++ {
		h.engine.bumpDelegateCount(run.ID)
	}
	if _, err := h.engine.delegate(ctx, run, "Ghost", "task", false); err == nil {
		t.Fatal("count bound not enforced")
	} else if err.Error() != "delegation count exceeded for this run" {
		t.Fatalf("count error %q", err)
	}
	// Depth bound.
	deep := ctx
	for i := 0; i < delegateMaxDepth; i++ {
		deep = delegateCtx(deep)
	}
	if _, err := h.engine.delegate(deep, run, "Ghost", "task", false); err == nil {
		t.Fatal("depth bound not enforced")
	} else if err.Error() != "delegation depth exceeded" {
		t.Fatalf("depth error %q", err)
	}
}

func TestDelegateCountStartsAtZero(t *testing.T) {
	h := newHarness(t, "never", nil)
	if got := h.engine.delegateCount("run_missing"); got != 0 {
		t.Fatalf("count %d, want 0", got)
	}
	h.engine.bumpDelegateCount("run_missing")
	if got := h.engine.delegateCount("run_missing"); got != 1 {
		t.Fatalf("count %d, want 1", got)
	}
}

func TestDelegateReadsPeerAnswer(t *testing.T) {
	h := newHarness(t, "never", [][]string{{contentChunk("peer answer here")}})
	ctx := context.Background()
	peer, err := h.store.CreateBot(ctx, "Peer", "Be brief.", "")
	if err != nil {
		t.Fatal(err)
	}
	parent, err := h.store.CreateRun(ctx, h.bot.ID, h.thread.ID, "user", "go")
	if err != nil {
		t.Fatal(err)
	}
	answer, err := h.engine.delegate(ctx, parent, peer.Name, "summarize this", false)
	if err != nil {
		t.Fatal(err)
	}
	if answer == "" || !strings.Contains(answer, "peer answer here") || !strings.Contains(answer, "Peer") {
		t.Fatalf("answer %q misses content or source", answer)
	}
	if got := h.engine.delegateCount(parent.ID); got != 1 {
		t.Fatalf("count %d, want 1", got)
	}
}
