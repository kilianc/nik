package apisvc

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/secrets"
)

func newTestSecrets(t *testing.T) *Secrets {
	t.Helper()

	dir, err := os.MkdirTemp("", "nik")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	return NewSecrets(secrets.New(dir))
}

func TestOwnerRoundTrip(t *testing.T) {
	store := newTestSecrets(t)
	ctx := context.Background()

	err := store.Set(ctx, api.ScopeOwner, "openai_key", "sk-test")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	value, err := store.Get(ctx, api.ScopeOwner, "openai_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if value != "sk-test" {
		t.Fatalf("value = %q", value)
	}
}

// The agent key signs and opens everything the platform holds for this nik.
// A skill holding it can impersonate this nik to the gateway, which is a
// different thing entirely from a skill holding an API key for a light bulb.
func TestSandboxIsDeniedNikOwnIdentity(t *testing.T) {
	store := newTestSecrets(t)
	ctx := context.Background()

	for _, name := range []string{"gateway_agent_key", "gateway_token"} {
		err := store.Set(ctx, api.ScopeOwner, name, "secret-value")
		if err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}

		_, err = store.Get(ctx, api.ScopeSandbox, name)
		if !errors.Is(err, api.ErrSecretDenied) {
			t.Errorf("Get(sandbox, %s) = %v, want ErrSecretDenied", name, err)
		}

		err = store.Set(ctx, api.ScopeSandbox, name, "overwritten")
		if !errors.Is(err, api.ErrSecretDenied) {
			t.Errorf("Set(sandbox, %s) = %v, want ErrSecretDenied", name, err)
		}

		// And the value survived the attempt.
		value, err := store.Get(ctx, api.ScopeOwner, name)
		if err != nil || value != "secret-value" {
			t.Errorf("%s = %q (%v) after a denied write", name, value, err)
		}
	}
}

// Case is not a way around the deny list.
func TestDenyListIgnoresCase(t *testing.T) {
	store := newTestSecrets(t)

	_, err := store.Get(context.Background(), api.ScopeSandbox, "Gateway_Agent_Key")
	if !errors.Is(err, api.ErrSecretDenied) {
		t.Fatalf("err = %v, want ErrSecretDenied for a differently-cased name", err)
	}
}

// Everything that is not nik's own identity is a credential nik holds on
// somebody's behalf, and a skill is what uses it.
func TestSandboxCanUseOrdinarySecrets(t *testing.T) {
	store := newTestSecrets(t)
	ctx := context.Background()

	err := store.Set(ctx, api.ScopeSandbox, "hue_token", "abc123")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	value, err := store.Get(ctx, api.ScopeSandbox, "hue_token")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if value != "abc123" {
		t.Fatalf("value = %q", value)
	}
}

func TestMissingSecretIsNotFound(t *testing.T) {
	store := newTestSecrets(t)

	_, err := store.Get(context.Background(), api.ScopeOwner, "never_written")
	if !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
