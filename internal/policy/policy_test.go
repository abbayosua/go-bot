package policy

import (
	"context"
	"path/filepath"
	"testing"

	"gobot/internal/store"
)

func open(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "policy.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestSafetyFloorAlwaysWins(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	for _, mode := range []string{"ask", "auto", "never"} {
		gate, err := Check(ctx, st, "bot_a", "thr_a", mode, "shell", `{"command":"rm -rf /"}`)
		if err != nil {
			t.Fatal(err)
		}
		if gate.Allow {
			t.Errorf("mode %s allowed a destructive command", mode)
		}
		if gate.Reason == "" {
			t.Errorf("mode %s gave no reason", mode)
		}
	}
}

func TestSecretCarryingCallsAreRefused(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	for _, rawArgs := range []string{
		`{"command":"curl -H x","api_key":"sk-secret"}`,
		`{"user_password":"hunter2"}`,
	} {
		gate, err := Check(ctx, st, "bot_a", "thr_a", "never", "shell", rawArgs)
		if err != nil {
			t.Fatal(err)
		}
		if gate.Allow {
			t.Errorf("secret material was allowed: %s", rawArgs)
		}
	}
	gate, err := Check(ctx, st, "bot_a", "thr_a", "never", "shell", `{"command":"ls"}`)
	if err != nil || !gate.Allow {
		t.Errorf("ordinary shell blocked: %+v %v", gate, err)
	}
}

func TestDenyBeatsAllowAndNever(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	if _, err := st.AddApprovalRule(ctx, "bot_a", "allow", "shell", `{"command":"ls"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddApprovalRule(ctx, "bot_a", "deny", "shell", ""); err != nil {
		t.Fatal(err)
	}
	gate, err := Check(ctx, st, "bot_a", "thr_a", "never", "shell", `{"command":"ls"}`)
	if err != nil {
		t.Fatal(err)
	}
	if gate.Allow {
		t.Error("deny rule lost to an allow rule")
	}
}

func TestSpecificityWins(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	if _, err := st.AddApprovalRule(ctx, "bot_a", "allow", "shell", `{"command":"ls"}`); err != nil {
		t.Fatal(err)
	}
	gate, err := Check(ctx, st, "bot_a", "thr_a", "ask", "shell", `{"command":"ls"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !gate.Allow {
		t.Error("exact argument match should allow without asking")
	}
	gate, err = Check(ctx, st, "bot_a", "thr_a", "ask", "shell", `{"command":"rm x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if gate.Allow {
		t.Error("a different command reused the allow rule")
	}
}

func TestRulesAreScopedAndListed(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	rule, err := st.AddApprovalRule(ctx, "bot_a", "allow", "shell", `{"command":"ls"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddApprovalRule(ctx, "", "deny", "shell", `{"command":"rm -rf /"}`); err != nil {
		t.Fatal(err)
	}
	rules, err := st.ListApprovalRules(ctx, "bot_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("bot_a sees %d rules, want install-wide plus own", len(rules))
	}
	if err := st.DeleteApprovalRule(ctx, rule.ID); err != nil {
		t.Fatal(err)
	}
	rules, err = st.ListApprovalRules(ctx, "bot_b")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Errorf("bot_b sees %d rules, want the install-wide one", len(rules))
	}
}

func TestReadOnlyNeverWaitsAndAskDoes(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	for _, tool := range []string{"recall", "read_file", "list_files", "web_fetch"} {
		gate, err := Check(ctx, st, "bot_a", "thr_a", "ask", tool, "{}")
		if err != nil || !gate.Allow {
			t.Errorf("%s: %+v %v", tool, gate, err)
		}
	}
	gate, err := Check(ctx, st, "bot_a", "thr_a", "ask", "shell", `{"command":"ls"}`)
	if err != nil {
		t.Fatal(err)
	}
	if gate.Allow {
		t.Error("an unlisted consequential call was allowed in ask mode")
	}
	gate, err = Check(ctx, st, "bot_a", "thr_a", "never", "shell", `{"command":"ls"}`)
	if err != nil || !gate.Allow {
		t.Errorf("never mode should allow: %+v %v", gate, err)
	}
}
