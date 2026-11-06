package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretsEncryptAtRest(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SetSecret(ctx, "API_TOKEN", "super-secret-value"); err != nil {
		t.Fatal(err)
	}
	// Raw database never reveals the value.
	var sealed []byte
	if err := st.db.QueryRowContext(ctx, `SELECT sealed FROM secret_blobs WHERE name = 'API_TOKEN'`).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), "super-secret") {
		t.Fatal("plaintext visible in blob")
	}
	rows, err := st.ListSecrets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "API_TOKEN" {
		t.Fatalf("list %+v", rows)
	}
	if !st.HasSecret(ctx, "API_TOKEN") || st.HasSecret(ctx, "MISSING") {
		t.Fatal("presence check wrong")
	}
	// UseSecret hands the value to fn and only fn.
	seen := ""
	out, err := st.UseSecret(ctx, "API_TOKEN", func(value string) (string, error) {
		seen = value
		return "used:" + value[:5], nil
	})
	if err != nil || out != "used:super" || seen != "super-secret-value" {
		t.Fatalf("use %q err %v", out, err)
	}
	if _, err := st.UseSecret(ctx, "MISSING", func(string) (string, error) { return "", nil }); err == nil {
		t.Fatal("missing secret usable")
	}
	if err := st.SetSecret(ctx, "", "x"); err == nil {
		t.Fatal("empty name stored")
	}
	if err := st.DeleteSecret(ctx, "API_TOKEN"); err != nil {
		t.Fatal(err)
	}
	if st.HasSecret(ctx, "API_TOKEN") {
		t.Fatal("deleted secret present")
	}
}
