package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestWalletCapsAndReceipts(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bot, err := st.CreateBot(ctx, "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if balance, err := st.Balance(ctx, bot.ID); err != nil || balance != 0 {
		t.Fatalf("fresh balance %d err %v", balance, err)
	}
	if _, err := st.TopUp(ctx, bot.ID, 0, ""); err == nil {
		t.Fatal("zero topup accepted")
	}
	balance, err := st.TopUp(ctx, bot.ID, 100, "grant")
	if err != nil || balance != 100 {
		t.Fatalf("topup %d err %v", balance, err)
	}
	if err := st.SetSetting(ctx, "wallet_cap:call", "10"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Charge(ctx, bot.ID, "call", 50, "over cap"); err == nil {
		t.Fatal("over-cap charge allowed")
	}
	if _, err := st.Charge(ctx, bot.ID, "", 5, ""); err == nil {
		t.Fatal("empty action charged")
	}
	if _, err := st.Charge(ctx, bot.ID, "call", 200, "over balance"); err == nil {
		t.Fatal("over-balance charge allowed")
	}
	receipt, err := st.Charge(ctx, bot.ID, "call", 8, "one call")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Amount != -8 || receipt.Balance != 92 || receipt.Action != "call" {
		t.Fatalf("receipt %+v", receipt)
	}
	receipts, err := st.ListReceipts(ctx, bot.ID, 10)
	if err != nil || len(receipts) != 2 {
		t.Fatalf("receipts %+v err %v", receipts, err)
	}
	if receipts[0].Action != "call" {
		t.Fatalf("order %+v", receipts)
	}
}
