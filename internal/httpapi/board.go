package httpapi

import (
	"net/http"
	"strings"
)

// listBoard returns the bot's kanban cards: todo, doing, done.
func (s *Server) listBoard(w http.ResponseWriter, r *http.Request) {
	cards, err := s.Store.ListBoardCards(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cards)
}

// createBoardCard appends a card to a column.
func (s *Server) createBoardCard(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	card, err := s.Store.AddBoardCard(r.Context(), r.PathValue("id"), strings.TrimSpace(body.Title), strings.TrimSpace(body.Status))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "board.add", card.ID, body.Title)
	writeJSON(w, http.StatusCreated, card)
}

// moveBoardCard moves a card between columns.
func (s *Server) moveBoardCard(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status   string `json:"status"`
		Position int    `json:"position"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	card, err := s.Store.MoveBoardCard(r.Context(), r.PathValue("id"), strings.TrimSpace(body.Status), body.Position)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "board.move", card.ID, body.Status)
	writeJSON(w, http.StatusOK, card)
}

// deleteBoardCard removes a card.
func (s *Server) deleteBoardCard(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteBoardCard(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "board.delete", r.PathValue("id"), "")
	w.WriteHeader(http.StatusNoContent)
}
