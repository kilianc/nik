package gateway

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

type memSecrets struct {
	m map[string]string
}

func newMemSecrets() *memSecrets { return &memSecrets{m: map[string]string{}} }

func (s *memSecrets) Get(name string) (string, error) {
	v, ok := s.m[name]
	if !ok {
		return "", errors.New("secret not found: " + name)
	}

	return v, nil
}

func (s *memSecrets) Set(name, value string) error {
	s.m[name] = value

	return nil
}

func TestLoadOrCreateKeyPersists(t *testing.T) {
	store := newMemSecrets()

	first, err := loadOrCreateKey(store)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	stored, ok := store.m[keySecretName]
	if !ok {
		t.Fatal("key not written to the secret store")
	}
	if len(stored) != keySize*2 {
		t.Errorf("stored key is %d hex chars, want %d", len(stored), keySize*2)
	}

	second, err := loadOrCreateKey(store)
	if err != nil {
		t.Fatalf("reload key: %v", err)
	}
	if *first != *second {
		t.Error("reload produced a different key — sealed backlog would be lost")
	}
}

func TestLoadOrCreateKeyRejectsCorrupt(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"not hex", "zz-not-hex"},
		{"wrong length", hex.EncodeToString([]byte("short"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemSecrets()
			store.m[keySecretName] = tt.value

			_, err := loadOrCreateKey(store)
			if err == nil {
				t.Fatal("corrupt key accepted")
			}
			if !strings.Contains(err.Error(), "corrupt key") {
				t.Errorf("err = %v", err)
			}
		})
	}
}
