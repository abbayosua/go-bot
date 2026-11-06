package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// User is one account. The first account created becomes the owner; later
// accounts start as viewers until an owner promotes them.
type User struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	CreatedAt   int64  `json:"createdAt"`
	PasswordSet bool   `json:"passwordSet"`
}

// Session is one login: token bearer, user agent recorded, revocable remotely.
type Session struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Token     string `json:"-"`
	UserAgent string `json:"userAgent"`
	CreatedAt int64  `json:"createdAt"`
}

const userSchema = `
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  role TEXT NOT NULL DEFAULT 'viewer',
  password_hash TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token TEXT NOT NULL UNIQUE,
  user_agent TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS space_roles (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  space_id TEXT NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'viewer',
  PRIMARY KEY (user_id, space_id)
);
`

const (
	RoleOwner  = "owner"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// passwordPolicy enforces the minimum: length, one digit, one non-letter.
func passwordPolicy(password string) error {
	if len(password) < 12 {
		return errors.New("password must be at least 12 characters")
	}
	var digit, other bool
	for _, r := range password {
		if r >= '0' && r <= '9' {
			digit = true
		}
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			other = true
		}
	}
	if !digit || !other {
		return errors.New("password needs a digit and a symbol or space")
	}
	return nil
}

func hashPassword(password, salt string) string {
	sum := sha256.Sum256([]byte("go-bot pw1:" + salt + ":" + password))
	for i := 0; i < 9999; i++ {
		next := sha256.Sum256(append(sum[:], []byte(password)...))
		sum = next
	}
	return hex.EncodeToString(sum[:])
}

func newToken(prefix string) string {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return NewID(prefix)
	}
	return prefix + hex.EncodeToString(raw[:])
}

// CreateUser makes one account. The first account is the owner.
func (s *Store) CreateUser(ctx context.Context, name, password string) (User, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return User{}, errors.New("name is required")
	}
	if err := passwordPolicy(password); err != nil {
		return User{}, err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return User{}, err
	}
	role := RoleViewer
	if count == 0 {
		role = RoleOwner
	}
	salt := newToken("sl_")
	user := User{ID: NewID("usr"), Name: name, Role: role, CreatedAt: time.Now().Unix(), PasswordSet: true}
	_, err := s.db.ExecContext(ctx, `INSERT INTO users (id, name, role, password_hash, created_at) VALUES (?, ?, ?, ?, ?)`,
		user.ID, user.Name, user.Role, salt+"$"+hashPassword(password, salt), user.CreatedAt)
	return user, err
}

// AuthUser checks a password and opens a session.
func (s *Store) AuthUser(ctx context.Context, name, password, userAgent string) (Session, error) {
	var id, stored string
	if err := s.db.QueryRowContext(ctx, `SELECT id, password_hash FROM users WHERE name = ?`, strings.TrimSpace(name)).Scan(&id, &stored); err != nil {
		return Session{}, ErrNotFound
	}
	salt, hash, ok := strings.Cut(stored, "$")
	if !ok || subtle.ConstantTimeCompare([]byte(hash), []byte(hashPassword(password, salt))) != 1 {
		return Session{}, ErrNotFound
	}
	session := Session{ID: NewID("ses"), UserID: id, Token: newToken("tok_"), UserAgent: userAgent, CreatedAt: time.Now().Unix()}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO sessions (id, user_id, token, user_agent, created_at) VALUES (?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.Token, session.UserAgent, session.CreatedAt); err != nil {
		return Session{}, err
	}
	return session, nil
}

// UserForToken resolves a session token to its user, or not found.
func (s *Store) UserForToken(ctx context.Context, token string) (User, Session, error) {
	var user User
	var session Session
	err := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.name, u.role, u.password_hash, u.created_at, s.id, s.user_agent, s.created_at
		 FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token = ?`, token).
		Scan(&user.ID, &user.Name, &user.Role, new(string), &user.CreatedAt, &session.ID, &session.UserAgent, &session.CreatedAt)
	if err != nil {
		return User{}, Session{}, ErrNotFound
	}
	user.PasswordSet = true
	session.UserID = user.ID
	return user, session, nil
}

// ListUsers returns every account, oldest first.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, role, created_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Name, &user.Role, &user.CreatedAt); err != nil {
			return nil, err
		}
		user.PasswordSet = true
		out = append(out, user)
	}
	return out, rows.Err()
}

// ListSessions returns one user's sessions (tokens hidden; ids allow revoke).
func (s *Store) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_agent, created_at FROM sessions WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var session Session
		session.UserID = userID
		if err := rows.Scan(&session.ID, &session.UserAgent, &session.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

// RevokeSession deletes one login: remote sign-out.
func (s *Store) RevokeSession(ctx context.Context, userID, sessionID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ? AND user_id = ?`, sessionID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetUserRole promotes or demotes (owners only, enforced in http).
func (s *Store) SetUserRole(ctx context.Context, userID, role string) error {
	if role != RoleOwner && role != RoleEditor && role != RoleViewer {
		return errors.New("role must be owner, editor or viewer")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE users SET role = ? WHERE id = ?`, role, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SpaceRole grants a user a role in one space.
func (s *Store) SpaceRole(ctx context.Context, userID, spaceID, role string) error {
	if role != RoleOwner && role != RoleEditor && role != RoleViewer {
		return errors.New("role must be owner, editor or viewer")
	}
	if _, err := s.GetUser(ctx, userID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO space_roles (user_id, space_id, role) VALUES (?, ?, ?)
		ON CONFLICT(user_id, space_id) DO UPDATE SET role = excluded.role`, userID, spaceID, role)
	return err
}

// GetUser returns one account.
func (s *Store) GetUser(ctx context.Context, id string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `SELECT id, name, role, created_at FROM users WHERE id = ?`, id).
		Scan(&user.ID, &user.Name, &user.Role, &user.CreatedAt)
	if err != nil {
		return User{}, ErrNotFound
	}
	user.PasswordSet = true
	return user, nil
}
