package httpapi

import (
	"net/http"
	"regexp"
	"strings"
)

// createGroup starts a shared chat: 2-10 bots, one thread, everyone separate.
func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SpaceID string   `json:"spaceId"`
		Members []string `json:"members"`
		Title   string   `json:"title"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = "Group chat"
	}
	group, thread, err := s.Store.CreateGroup(r.Context(), strings.TrimSpace(body.SpaceID), body.Members, title)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "group.create", group.ID, title)
	writeJSON(w, http.StatusCreated, map[string]any{"group": group, "thread": thread})
}

// listGroups returns one space's groups.
func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.Store.ListGroups(r.Context(), strings.TrimSpace(r.URL.Query().Get("spaceId")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

var mentionPattern = regexp.MustCompile(`@([A-Za-z0-9_][A-Za-z0-9_.-]*)`)

// mentionMembers matches @names in a message to member ids. Names match
// case-insensitively on the full name or any word of it, so "@ana" reaches
// "Ana S". Unknown mentions are ignored; no match means everyone answers.
func mentionMembers(content string, names map[string]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, match := range mentionPattern.FindAllStringSubmatch(content, -1) {
		want := strings.ToLower(match[1])
		for id, name := range names {
			if seen[id] {
				continue
			}
			lowered := strings.ToLower(name)
			hit := lowered == want
			if !hit {
				for _, word := range strings.FieldsFunc(lowered, func(r rune) bool {
					return r == ' ' || r == '-' || r == '_' || r == '.'
				}) {
					if word == want {
						hit = true
						break
					}
				}
			}
			if hit {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}
