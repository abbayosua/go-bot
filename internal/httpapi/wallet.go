package httpapi

import (
	"net/http"
	"strings"
)

// walletBalance returns one bot's credits plus recent receipts.
func (s *Server) walletBalance(w http.ResponseWriter, r *http.Request) {
	botID := r.PathValue("id")
	balance, err := s.Store.Balance(r.Context(), botID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	receipts, err := s.Store.ListReceipts(r.Context(), botID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"balance": balance, "receipts": receipts})
}

// walletTopUp adds credits (owners and editors; viewers read only).
func (s *Server) walletTopUp(w http.ResponseWriter, r *http.Request) {
	if !s.canSpend(r) {
		writeError(w, http.StatusForbidden, "editors only")
		return
	}
	var body struct {
		Amount int64  `json:"amount"`
		Note   string `json:"note"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	balance, err := s.Store.TopUp(r.Context(), r.PathValue("id"), body.Amount, strings.TrimSpace(body.Note))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "wallet.topup", r.PathValue("id"), "")
	writeJSON(w, http.StatusOK, map[string]any{"balance": balance})
}

// walletCharge draws credits through the caps (agents use the store method;
// the endpoint exists for the admin panel and tests).
func (s *Server) walletCharge(w http.ResponseWriter, r *http.Request) {
	if !s.canSpend(r) {
		writeError(w, http.StatusForbidden, "editors only")
		return
	}
	var body struct {
		Action string `json:"action"`
		Amount int64  `json:"amount"`
		Note   string `json:"note"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	receipt, err := s.Store.Charge(r.Context(), r.PathValue("id"), body.Action, body.Amount, strings.TrimSpace(body.Note))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "wallet.charge", r.PathValue("id"), body.Action)
	writeJSON(w, http.StatusOK, receipt)
}

func (s *Server) canSpend(r *http.Request) bool {
	if users, err := s.Store.ListUsers(r.Context()); err != nil || len(users) == 0 {
		return true
	}
	user, _, ok := s.currentUser(r)
	return ok && (user.Role == "owner" || user.Role == "editor")
}
