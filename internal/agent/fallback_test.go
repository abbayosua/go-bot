package agent

import "testing"

func TestModelChainDedupesAndSkipsBlanks(t *testing.T) {
	chain := modelChain("a", []string{"", "b", "a", "c"})
	want := []string{"a", "b", "c"}
	if len(chain) != len(want) {
		t.Fatalf("chain %v, want %v", chain, want)
	}
	for i := range want {
		if chain[i] != want[i] {
			t.Fatalf("chain %v, want %v", chain, want)
		}
	}
}
