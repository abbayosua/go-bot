package agent

import "strings"

// splitFallbacks parses the comma-separated fallback list.

// modelChain returns the models to try for one role: the primary first,
// then each configured fallback that is not blank or a duplicate.
func modelChain(primary string, fallbacks []string) []string {
	chain := []string{}
	seen := map[string]bool{}
	for _, model := range append([]string{primary}, fallbacks...) {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		chain = append(chain, model)
	}
	return chain
}

// splitFallbacks parses the comma-separated fallback list.
func splitFallbacks(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
