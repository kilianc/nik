package apisvc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/secrets"
)

// Secrets serves the encrypted store, and decides who may have what.
//
// The deciding is the new part. Until now the store was a pair of files inside
// a workspace the shell container mounts read-write, so a skill wanting a
// credential did not need permission — it needed a path. Now the sandbox asks,
// and some answers are no.
type Secrets struct {
	store *secrets.Store
}

func NewSecrets(store *secrets.Store) *Secrets {
	return &Secrets{store: store}
}

// sandboxDenied are the secrets a skill in the shell container may never read
// or write, whatever it claims to need them for.
//
// They are nik's own identity rather than a credential nik uses on somebody's
// behalf. gateway_agent_key signs and opens everything the platform holds for
// this agent; gateway_token is the account link. A skill holding either can
// impersonate this nik to the gateway, which is a different thing entirely
// from a skill holding an API key for a light bulb.
var sandboxDenied = map[string]bool{
	"gateway_agent_key": true,
	"gateway_token":     true,
}

func (s *Secrets) List(ctx context.Context) ([]string, error) {
	names, err := s.store.List()
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}

	return names, nil
}

func (s *Secrets) Get(ctx context.Context, scope api.Scope, name string) (string, error) {
	err := s.permit(scope, name, "read")
	if err != nil {
		return "", err
	}

	value, err := s.store.Get(name)
	if err != nil {
		// The store does not distinguish a missing secret from a broken one,
		// and neither answer should tell a caller which names exist.
		return "", api.ErrNotFound
	}

	return value, nil
}

func (s *Secrets) Set(ctx context.Context, scope api.Scope, name, value string) error {
	err := s.permit(scope, name, "write")
	if err != nil {
		return err
	}

	err = s.store.Set(name, value)
	if err != nil {
		return fmt.Errorf("%w: %s", api.ErrInvalidField, err)
	}

	return nil
}

func (s *Secrets) Delete(ctx context.Context, name string) error {
	err := s.store.Delete(name)
	if err != nil {
		return api.ErrNotFound
	}

	return nil
}

// permit logs what it refuses. The caller is told only "no such secret", so
// the daemon's log is the only place the attempt is visible — and a skill
// reaching for the agent key is worth seeing.
func (s *Secrets) permit(scope api.Scope, name, action string) error {
	if scope != api.ScopeSandbox {
		return nil
	}

	if sandboxDenied[strings.ToLower(name)] {
		slog.Warn("sandbox denied a secret",
			"pkg", "apisvc", "name", name, "action", action)

		return api.ErrSecretDenied
	}

	return nil
}
