package auth

import (
	"context"
	"testing"
)

func TestConfigAvailability(t *testing.T) {
	if (Config{}).Available() {
		t.Fatal("empty config available")
	}
	if !(Config{Passkey: true}).Available() {
		t.Fatal("passkey not available")
	}
	if !(Config{OIDC: "agents", Issuer: "https://x.example"}).Available() {
		t.Fatal("oidc not available")
	}
	if ErrNotConfigured == nil {
		t.Fatal("missing sentinel")
	}
	var _ Provider = (*stub)(nil)
	if _, err := (*stub)(nil).Verify(context.Background(), nil); err == nil {
		t.Fatal("stub verifies")
	}
}

type stub struct{}

func (s *stub) Name() string { return "stub" }

func (s *stub) Verify(_ context.Context, _ []byte) (ExternalUser, error) {
	return ExternalUser{}, ErrNotConfigured
}
