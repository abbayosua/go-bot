package store

import (
	"context"
	"errors"
	"strings"
)

// Wallet is a prepaid balance the agent draws from for paid actions.
// Amounts are integer credits (1 credit = smallest billable unit the operator
// defines). Caps bound one action; receipts record every movement.
type Receipt struct {
	ID        string `json:"id"`
	BotID     string `json:"botId"`
	Action    string `json:"action"`
	Amount    int64  `json:"amount"`
	Balance   int64  `json:"balance"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"createdAt"`
}

const walletSchema = `
CREATE TABLE IF NOT EXISTS wallets (
  bot_id TEXT PRIMARY KEY REFERENCES bots(id) ON DELETE CASCADE,
  balance INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS receipts (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  action TEXT NOT NULL,
  amount INTEGER NOT NULL,
  balance INTEGER NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS receipts_bot ON receipts(bot_id, created_at DESC);
`

// Balance returns one bot's credits, defaulting to zero.
func (s *Store) Balance(ctx context.Context, botID string) (int64, error) {
	var balance int64
	err := s.db.QueryRowContext(ctx, `SELECT balance FROM wallets WHERE bot_id = ?`, botID).Scan(&balance)
	if err != nil {
		return 0, nil
	}
	return balance, nil
}

// TopUp adds credits; receipts record it as action "topup".
func (s *Store) TopUp(ctx context.Context, botID string, amount int64, note string) (int64, error) {
	if amount <= 0 {
		return 0, errors.New("amount must be positive")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO wallets (bot_id, balance) VALUES (?, 0) ON CONFLICT(bot_id) DO NOTHING`, botID); err != nil {
		return 0, err
	}
	var balance int64
	if err := tx.QueryRowContext(ctx, `SELECT balance FROM wallets WHERE bot_id = ?`, botID).Scan(&balance); err != nil {
		return 0, err
	}
	balance += amount
	if _, err := tx.ExecContext(ctx, `UPDATE wallets SET balance = ? WHERE bot_id = ?`, balance, botID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO receipts (id, bot_id, action, amount, balance, note, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		NewID("rcp"), botID, "topup", amount, balance, note, now()); err != nil {
		return 0, err
	}
	return balance, tx.Commit()
}

// Charge draws credits for one action, enforcing the per-action cap
// (setting "wallet_cap:<action>", 0 means uncapped) and the balance.
func (s *Store) Charge(ctx context.Context, botID, action string, amount int64, note string) (Receipt, error) {
	action = strings.TrimSpace(action)
	if action == "" || amount <= 0 {
		return Receipt{}, errors.New("action and a positive amount are required")
	}
	cap := int64(0)
	if raw, err := s.GetSetting(ctx, "wallet_cap:"+action); err == nil && strings.TrimSpace(raw) != "" {
		var parsed int64
		if _, err := splitInt(raw, &parsed); err == nil && parsed > 0 {
			cap = parsed
		}
	}
	if cap > 0 && amount > cap {
		return Receipt{}, errors.New("amount exceeds the per-action cap")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Receipt{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO wallets (bot_id, balance) VALUES (?, 0) ON CONFLICT(bot_id) DO NOTHING`, botID); err != nil {
		return Receipt{}, err
	}
	var balance int64
	if err := tx.QueryRowContext(ctx, `SELECT balance FROM wallets WHERE bot_id = ?`, botID).Scan(&balance); err != nil {
		return Receipt{}, err
	}
	if balance < amount {
		return Receipt{}, errors.New("insufficient balance")
	}
	balance -= amount
	if _, err := tx.ExecContext(ctx, `UPDATE wallets SET balance = ? WHERE bot_id = ?`, balance, botID); err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{ID: NewID("rcp"), BotID: botID, Action: action, Amount: -amount, Balance: balance, Note: note, CreatedAt: now()}
	if _, err := tx.ExecContext(ctx, `INSERT INTO receipts (id, bot_id, action, amount, balance, note, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		receipt.ID, receipt.BotID, receipt.Action, receipt.Amount, receipt.Balance, receipt.Note, receipt.CreatedAt); err != nil {
		return Receipt{}, err
	}
	return receipt, tx.Commit()
}

// ListReceipts returns one bot's receipts, newest first.
func (s *Store) ListReceipts(ctx context.Context, botID string, limit int) ([]Receipt, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, bot_id, action, amount, balance, note, created_at FROM receipts WHERE bot_id = ? ORDER BY created_at DESC, rowid DESC LIMIT ?`, botID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Receipt{}
	for rows.Next() {
		var receipt Receipt
		if err := rows.Scan(&receipt.ID, &receipt.BotID, &receipt.Action, &receipt.Amount, &receipt.Balance, &receipt.Note, &receipt.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, receipt)
	}
	return out, rows.Err()
}

func splitInt(raw string, out *int64) (int, error) {
	var n int64
	count := 0
	for _, r := range strings.TrimSpace(raw) {
		if r < '0' || r > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int64(r-'0')
		count++
	}
	if count == 0 {
		return 0, errors.New("not a number")
	}
	*out = n
	return count, nil
}
