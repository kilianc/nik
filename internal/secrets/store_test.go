package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetAndGet(t *testing.T) {
	s := New(t.TempDir())

	err := s.Set("openai_key", "sk-test-123")
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	val, err := s.Get("openai_key")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if val != "sk-test-123" {
		t.Fatalf("expected sk-test-123, got %s", val)
	}
}

func TestGetNotFound(t *testing.T) {
	s := New(t.TempDir())

	_, err := s.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing secret")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestSetOverwrite(t *testing.T) {
	s := New(t.TempDir())

	err := s.Set("key", "v1")
	if err != nil {
		t.Fatalf("set v1: %v", err)
	}

	err = s.Set("key", "v2")
	if err != nil {
		t.Fatalf("set v2: %v", err)
	}

	val, err := s.Get("key")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if val != "v2" {
		t.Fatalf("expected v2, got %s", val)
	}
}

func TestDelete(t *testing.T) {
	s := New(t.TempDir())

	err := s.Set("key", "value")
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	err = s.Delete("key")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = s.Get("key")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := New(t.TempDir())

	err := s.Delete("nonexistent")
	if err == nil {
		t.Fatal("expected error deleting nonexistent secret")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestList(t *testing.T) {
	s := New(t.TempDir())

	names, err := s.List()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected empty list, got %v", names)
	}

	err = s.Set("bravo", "2")
	if err != nil {
		t.Fatalf("set bravo: %v", err)
	}

	err = s.Set("alpha", "1")
	if err != nil {
		t.Fatalf("set alpha: %v", err)
	}

	names, err = s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(names) != 2 || names[0] != "alpha" || names[1] != "bravo" {
		t.Fatalf("expected [alpha bravo], got %v", names)
	}
}

func TestNameValidation(t *testing.T) {
	s := New(t.TempDir())

	tests := []struct {
		name    string
		wantErr bool
	}{
		{"openai_key", false},
		{"exa-api-key", false},
		{"KEY123", false},
		{"", true},
		{"has.dot", true},
		{"has/slash", true},
		{"has space", true},
		{"../traversal", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.Set(tt.name, "val")
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for name %q", tt.name)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for name %q: %v", tt.name, err)
			}
		})
	}
}

func TestKeyAutoGeneration(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	keyPath := filepath.Join(dir, "secrets", "secrets.key")
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatal("key file should not exist before first write")
	}

	err := s.Set("test", "value")
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("key file should exist after first write: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key file perms: expected 0600, got %o", info.Mode().Perm())
	}

	if info.Size() != keySize {
		t.Fatalf("key file size: expected %d, got %d", keySize, info.Size())
	}
}

func TestEncryptionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	err := s.Set("secret", "hunter2")
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "secrets", "secrets.enc"))
	if err != nil {
		t.Fatalf("read enc file: %v", err)
	}

	if strings.Contains(string(raw), "hunter2") {
		t.Fatal("plaintext value found in encrypted file")
	}

	if strings.Contains(string(raw), "secret") {
		t.Fatal("plaintext key name found in encrypted file")
	}

	val, err := s.Get("secret")
	if err != nil {
		t.Fatalf("get after encryption: %v", err)
	}

	if val != "hunter2" {
		t.Fatalf("expected hunter2, got %s", val)
	}
}

func TestWrongKeyFails(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	err := s.Set("key", "value")
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	err = os.WriteFile(s.keyPath, make([]byte, keySize), 0o600)
	if err != nil {
		t.Fatalf("overwrite key: %v", err)
	}

	_, err = s.Get("key")
	if err == nil {
		t.Fatal("expected error with wrong key")
	}

	if !strings.Contains(err.Error(), "decrypt failed") {
		t.Fatalf("expected decrypt error, got: %v", err)
	}
}

func TestMultipleSecrets(t *testing.T) {
	s := New(t.TempDir())

	secrets := map[string]string{
		"openai_key":    "sk-openai",
		"anthropic_key": "sk-ant",
		"exa_api_key":   "exa-123",
	}

	for k, v := range secrets {
		err := s.Set(k, v)
		if err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
	}

	for k, want := range secrets {
		got, err := s.Get(k)
		if err != nil {
			t.Fatalf("get %s: %v", k, err)
		}
		if got != want {
			t.Fatalf("get %s: expected %s, got %s", k, want, got)
		}
	}

	names, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
}

func TestDataFilePermissions(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	err := s.Set("test", "value")
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "secrets", "secrets.enc"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Fatalf("data file perms: expected 0600, got %o", info.Mode().Perm())
	}
}

func TestEnsureAdapter(t *testing.T) {
	t.Run("copies adapter when missing", func(t *testing.T) {
		home := t.TempDir()
		skillsDir := t.TempDir()

		err := os.MkdirAll(filepath.Join(skillsDir, "secrets"), 0o755)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		err = os.WriteFile(filepath.Join(skillsDir, "secrets", "cli.sh"), []byte("#!/bin/sh\necho ok"), 0o644)
		if err != nil {
			t.Fatalf("write source: %v", err)
		}

		EnsureAdapter(home, skillsDir)

		dst := filepath.Join(home, "secrets", "cli")
		info, err := os.Stat(dst)
		if err != nil {
			t.Fatalf("adapter not created: %v", err)
		}

		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("adapter not executable: %o", info.Mode().Perm())
		}

		data, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("read adapter: %v", err)
		}

		if string(data) != "#!/bin/sh\necho ok" {
			t.Fatalf("adapter content mismatch: %q", data)
		}
	})

	t.Run("skips when adapter exists", func(t *testing.T) {
		home := t.TempDir()
		skillsDir := t.TempDir()

		err := os.MkdirAll(filepath.Join(home, "secrets"), 0o755)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		err = os.WriteFile(filepath.Join(home, "secrets", "cli"), []byte("custom"), 0o755)
		if err != nil {
			t.Fatalf("write existing: %v", err)
		}

		EnsureAdapter(home, skillsDir)

		data, err := os.ReadFile(filepath.Join(home, "secrets", "cli"))
		if err != nil {
			t.Fatalf("read: %v", err)
		}

		if string(data) != "custom" {
			t.Fatal("existing adapter was overwritten")
		}
	})

	t.Run("no-op when source missing", func(t *testing.T) {
		home := t.TempDir()
		skillsDir := t.TempDir()

		EnsureAdapter(home, skillsDir)

		_, err := os.Stat(filepath.Join(home, "secrets", "cli"))
		if err == nil {
			t.Fatal("adapter should not exist when source is missing")
		}
	})
}
