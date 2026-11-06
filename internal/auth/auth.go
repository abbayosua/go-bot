// Package auth holds the external login contracts: passkeys (WebAuthn) and
// upstream OIDC. The password path in internal/store is the default; these
// adapters plug in behind the same session issue when an operator configures
// them. No stub ever mints a session on its own.
package auth

import (
	"context"
	"errors"
)

// ExternalUser is an identity asserted by an outside provider.
type ExternalUser struct {
	Subject string
	Name    string
	Email   string
}

// Provider verifies an outside credential and returns who it is.
type Provider interface {
	// Name identifies the provider in logs and the UI.
	Name() string
	// Verify checks a credential and returns the asserted identity.
	Verify(ctx context.Context, credential []byte) (ExternalUser, error)
}

// Config names the configured providers, if any.
type Config struct {
	Passkey bool
	OIDC    string
	Issuer  string
}

// Available reports whether any external provider is configured.
func (c Config) Available() bool { return c.Passkey || c.OIDC != "" }

// ErrNotConfigured is returned when no external provider is set up.
var ErrNotConfigured = errors.New("no external auth provider configured")
