package memory

import (
	"context"
	"fmt"
	"strings"

	"gobot/internal/store"
)

// Budgets keep injections small: the model sees newest items first, and prose
// never drowns the conversation.
const (
	MaxMemoryItems = 30
	MaxMemoryChars = 4000
	MaxSkillChars  = 6000
)

// Block assembles the shared and bot-scoped facts injected into every system
// prompt. Shared items come first; newest items come first within each scope.
func Block(ctx context.Context, st *store.Store, botID string) string {
	memories, err := st.ListMemories(ctx, botID, MaxMemoryItems)
	if err != nil {
		return ""
	}
	var shared, personal []store.Memory
	for _, memory := range memories {
		if memory.BotID == "" {
			shared = append(shared, memory)
		} else {
			personal = append(personal, memory)
		}
	}
	var b strings.Builder
	write := func(title string, items []store.Memory) {
		for _, memory := range items {
			line := "- " + strings.TrimSpace(memory.Content) + "\n"
			if b.Len()+len(line) > MaxMemoryChars {
				return
			}
			if b.Len() == 0 {
				b.WriteString(title + "\n")
			} else if strings.HasSuffix(b.String(), "\n\n") {
				b.WriteString(strings.TrimPrefix(title, "\n"))
			}
			b.WriteString(line)
		}
	}
	write("\nShared facts:\n", shared)
	if len(personal) > 0 && b.Len() > 0 {
		b.WriteString("\n")
	}
	write("\nFacts about this bot's work:\n", personal)
	return strings.TrimSpace(b.String())
}

// SkillNames lists what `/name` can invoke.
func SkillNames(skills []store.Skill) string {
	if len(skills) == 0 {
		return ""
	}
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, "/"+skill.Name)
	}
	return strings.Join(names, ", ")
}

// Invoke returns the playbook and knowledge entries for a `/skill` command.
// The user line doubles as the search query, so "no matches" is an explicit
// answer rather than silence.
func Invoke(ctx context.Context, st *store.Store, botID, command, query string) (string, error) {
	name := strings.TrimPrefix(command, "/")
	skill, err := st.GetSkill(ctx, name)
	if err != nil {
		available, listErr := st.ListSkills(ctx)
		if listErr != nil {
			return "", listErr
		}
		return fmt.Sprintf("Unknown skill %q. Available: %s", command, SkillNames(available)), nil
	}
	body := strings.TrimSpace(skill.Body)
	if len(body) > MaxSkillChars {
		body = body[:MaxSkillChars] + "\n[playbook truncated]"
	}
	var b strings.Builder
	b.WriteString("Skill " + skill.Name + ":\n" + body)
	if query != "" {
		entries, err := st.SearchKnowledge(ctx, botID, query, 3)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			b.WriteString("\n\nRelated knowledge — " + entry.Title + ":\n" + entry.Body)
		}
		if len(entries) == 0 {
			b.WriteString("\n\nNo matching knowledge entries.")
		}
	}
	return b.String(), nil
}
