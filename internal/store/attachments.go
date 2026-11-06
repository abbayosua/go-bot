package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
)

// Attachment metadata: the bytes live encrypted in attachment_blobs, keyed by
// the same id. The database file alone never reveals content.
type Attachment struct {
	ID          string `json:"id"`
	ThreadID    string `json:"threadId"`
	SessionID   string `json:"-"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	CreatedAt   int64  `json:"createdAt"`
}

const attachmentSchema = `
CREATE TABLE IF NOT EXISTS attachments (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL DEFAULT '',
  filename TEXT NOT NULL,
  content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
  size INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS attachment_blobs (
  id TEXT PRIMARY KEY REFERENCES attachments(id) ON DELETE CASCADE,
  nonce BLOB NOT NULL,
  sealed BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS attachments_thread ON attachments(thread_id, created_at);
`

// sessionSaltKey persists the install salt used to wrap attachment keys.
const sessionSaltKey = "attachment_session_salt"

func newSessionID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return NewID("ses")
	}
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 3+len(raw)*2)
	out[0], out[1], out[2] = 's', 's', ':'
	for i, b := range raw {
		out[3+i*2] = hexdigits[b>>4]
		out[3+i*2+1] = hexdigits[b&0x0f]
	}
	return string(out)
}

// SessionSalt returns the persistent install salt, creating it on first use.
func (s *Store) SessionSalt(ctx context.Context) ([]byte, error) {
	if existing, err := s.GetSetting(ctx, sessionSaltKey); err == nil && existing != "" {
		return []byte(existing), nil
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, err
	}
	salt := "ss:" + string(raw[:])
	if err := s.SetSetting(ctx, sessionSaltKey, salt); err != nil {
		return nil, err
	}
	return []byte(salt), nil
}

// deriveKey stretches a session bearer into a 32-byte box key with
// HKDF-SHA256: session key material, install salt, attachment context.
func deriveKey(session string, salt []byte) []byte {
	prk := hmac.New(sha256.New, salt)
	prk.Write([]byte(session))
	okm := hmac.New(sha256.New, prk.Sum(nil))
	okm.Write([]byte("go-bot attachment v1"))
	okm.Write([]byte{0x01})
	sum := okm.Sum(nil)
	key := make([]byte, 32)
	copy(key, sum)
	return key
}

var errNoSession = errors.New("attachment session is required")

func sealAttachment(key, plaintext []byte) (nonce, sealed []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, gcm.Seal(nil, nonce, plaintext, nil), nil
}

func openAttachment(key, nonce, sealed []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, sealed, nil)
}

// SaveAttachment encrypts content and stores metadata plus blob.
func (s *Store) SaveAttachment(ctx context.Context, threadID, session, filename, contentType string, content []byte) (Attachment, error) {
	if session == "" {
		return Attachment{}, errNoSession
	}
	if _, err := s.GetThread(ctx, threadID); err != nil {
		return Attachment{}, err
	}
	if len(content) > 10<<20 {
		return Attachment{}, errors.New("attachment over 10 MiB")
	}
	salt, err := s.SessionSalt(ctx)
	if err != nil {
		return Attachment{}, err
	}
	nonce, sealed, err := sealAttachment(deriveKey(session, salt), content)
	if err != nil {
		return Attachment{}, err
	}
	attachment := Attachment{
		ID: NewID("att"), ThreadID: threadID, SessionID: session,
		Filename: filename, ContentType: contentType,
		Size:      int64(len(content)),
		CreatedAt: now(),
	}
	if attachment.ContentType == "" {
		attachment.ContentType = "application/octet-stream"
	}
	return s.saveAttachmentTx(ctx, attachment, nonce, sealed)
}

func (s *Store) saveAttachmentTx(ctx context.Context, attachment Attachment, nonce, sealed []byte) (Attachment, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Attachment{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO attachments (id, thread_id, session_id, filename, content_type, size, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		attachment.ID, attachment.ThreadID, attachment.SessionID, attachment.Filename, attachment.ContentType, attachment.Size, attachment.CreatedAt); err != nil {
		return Attachment{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO attachment_blobs (id, nonce, sealed) VALUES (?, ?, ?)`,
		attachment.ID, nonce, sealed); err != nil {
		return Attachment{}, err
	}
	return attachment, tx.Commit()
}

// OpenAttachment decrypts one blob, but only for the session that uploaded it.
func (s *Store) OpenAttachment(ctx context.Context, id, session string) (Attachment, []byte, error) {
	var attachment Attachment
	var nonce, sealed []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT a.id, a.thread_id, a.session_id, a.filename, a.content_type, a.size, a.created_at, b.nonce, b.sealed
		 FROM attachments a JOIN attachment_blobs b ON b.id = a.id WHERE a.id = ?`, id).
		Scan(&attachment.ID, &attachment.ThreadID, &attachment.SessionID, &attachment.Filename,
			&attachment.ContentType, &attachment.Size, &attachment.CreatedAt, &nonce, &sealed)
	if err != nil {
		return Attachment{}, nil, ErrNotFound
	}
	if session == "" || subtleMismatch(attachment.SessionID, session) {
		return Attachment{}, nil, errNoSession
	}
	salt, err := s.SessionSalt(ctx)
	if err != nil {
		return Attachment{}, nil, err
	}
	plaintext, err := openAttachment(deriveKey(session, salt), nonce, sealed)
	if err != nil {
		return Attachment{}, nil, err
	}
	attachment.SessionID = ""
	return attachment, plaintext, nil
}

func subtleMismatch(a, b string) bool {
	if len(a) != len(b) {
		return true
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff != 0
}

// ListAttachments returns one thread's attachment metadata (never content).
func (s *Store) ListAttachments(ctx context.Context, threadID string) ([]Attachment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, thread_id, filename, content_type, size, created_at FROM attachments
		 WHERE thread_id = ? ORDER BY created_at`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attachment{}
	for rows.Next() {
		var attachment Attachment
		if err := rows.Scan(&attachment.ID, &attachment.ThreadID, &attachment.Filename,
			&attachment.ContentType, &attachment.Size, &attachment.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, attachment)
	}
	return out, rows.Err()
}
