package httpapi

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
)

// auditActor names the caller: the token tail when a token is set, else local.
func (s *Server) auditActor(r *http.Request) string {
	if s.Token == "" {
		return "local"
	}
	if len(s.Token) > 4 {
		return "token:…" + s.Token[len(s.Token)-4:]
	}
	return "token"
}

func (s *Server) audit(r *http.Request, action, target, detail string) {
	_ = s.Store.AddAudit(r.Context(), s.auditActor(r), action, target, detail)
}

// listAudit returns audit rows newest first; ?format=csv exports them.
func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	rows, err := s.Store.ListAudit(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if strings.ToLower(r.URL.Query().Get("format")) == "csv" {
		var out strings.Builder
		writer := csv.NewWriter(&out)
		_ = writer.Write([]string{"id", "actor", "action", "target", "detail", "created_at"})
		for _, row := range rows {
			_ = writer.Write([]string{row.ID, row.Actor, row.Action, row.Target, row.Detail, strconv.FormatInt(row.CreatedAt, 10)})
		}
		writer.Flush()
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=audit.csv")
		_, _ = w.Write([]byte(out.String()))
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
