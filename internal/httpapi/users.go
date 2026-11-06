package httpapi

import (
	"net/http"
	"strings"

	"gobot/internal/store"
)

// currentUser resolves the session behind the request, if multi-user is on.
func (s *Server) currentUser(r *http.Request) (store.User, store.Session, bool) {
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if provided == "" {
		provided = r.URL.Query().Get("token")
	}
	if provided == "" {
		return store.User{}, store.Session{}, false
	}
	user, session, err := s.Store.UserForToken(r.Context(), provided)
	if err != nil {
		return store.User{}, store.Session{}, false
	}
	return user, session, true
}

// multiUserOn reports whether accounts exist: then sessions rule.
func (s *Server) multiUserOn(r *http.Request) bool {
	users, err := s.Store.ListUsers(r.Context())
	return err == nil && len(users) > 0
}

// register creates one account; the first becomes the owner.
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	user, err := s.Store.CreateUser(r.Context(), body.Name, body.Password)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "user.register", user.ID, user.Name)
	writeJSON(w, http.StatusCreated, user)
}

// login checks the password and returns a session token.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	session, err := s.Store.AuthUser(r.Context(), body.Name, body.Password, r.UserAgent())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "bad credentials")
		return
	}
	token := session.Token
	session.Token = ""
	s.audit(r, "user.login", session.UserID, "")
	writeJSON(w, http.StatusOK, map[string]any{"session": session, "token": token})
}

// me returns the caller and their sessions (remote sign-out targets).
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, _, ok := s.currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	sessions, err := s.Store.ListSessions(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "sessions": sessions})
}

// logout revokes the caller's session.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	user, session, ok := s.currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	if err := s.Store.RevokeSession(r.Context(), user.ID, session.ID); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// revokeSession signs one session out remotely (the caller's own).
func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	user, _, ok := s.currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	if err := s.Store.RevokeSession(r.Context(), user.ID, r.PathValue("id")); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "session.revoke", r.PathValue("id"), "")
	w.WriteHeader(http.StatusNoContent)
}

// listUsers returns accounts (owners only).
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	user, _, ok := s.currentUser(r)
	if !ok || user.Role != store.RoleOwner {
		writeError(w, http.StatusForbidden, "owners only")
		return
	}
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

// setUserRole promotes or demotes (owners only).
func (s *Server) setUserRole(w http.ResponseWriter, r *http.Request) {
	caller, _, ok := s.currentUser(r)
	if !ok || caller.Role != store.RoleOwner {
		writeError(w, http.StatusForbidden, "owners only")
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.Store.SetUserRole(r.Context(), r.PathValue("id"), strings.TrimSpace(body.Role)); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "user.role", r.PathValue("id"), body.Role)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// setSpaceRole grants a role in one space (owners only).
func (s *Server) setSpaceRole(w http.ResponseWriter, r *http.Request) {
	caller, _, ok := s.currentUser(r)
	if !ok || caller.Role != store.RoleOwner {
		writeError(w, http.StatusForbidden, "owners only")
		return
	}
	var body struct {
		UserID string `json:"userId"`
		Role   string `json:"role"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.Store.SpaceRole(r.Context(), body.UserID, r.PathValue("id"), strings.TrimSpace(body.Role)); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	s.audit(r, "space.role", r.PathValue("id"), body.Role)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
