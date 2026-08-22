package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".codex", "auth.json")

	original := &Auth{
		AccountID:    "acct-123",
		accessToken:  "access-tok",
		refreshToken: "refresh-tok",
		expiresAt:    time.Now().Add(time.Hour).Truncate(time.Second),
		filePath:     path,
	}

	err := original.save()
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.AccountID != original.AccountID {
		t.Errorf("AccountID = %q, want %q", loaded.AccountID, original.AccountID)
	}
	if loaded.accessToken != original.accessToken {
		t.Errorf("accessToken = %q, want %q", loaded.accessToken, original.accessToken)
	}
	if loaded.refreshToken != original.refreshToken {
		t.Errorf("refreshToken = %q, want %q", loaded.refreshToken, original.refreshToken)
	}
	if loaded.expiresAt.Sub(original.expiresAt).Abs() > time.Second {
		t.Errorf("expiresAt = %v, want %v", loaded.expiresAt, original.expiresAt)
	}
}

func TestLoadCodexCLIFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	af := authFile{}
	af.Tokens.AccessToken = "cli-token"
	af.Tokens.RefreshToken = "cli-refresh"
	af.Tokens.AccountID = "cli-acct"

	data, err := json.Marshal(af)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	err = os.WriteFile(path, data, 0o600)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	auth, err := load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if auth.accessToken != "cli-token" {
		t.Errorf("accessToken = %q, want %q", auth.accessToken, "cli-token")
	}
	if auth.AccountID != "cli-acct" {
		t.Errorf("AccountID = %q, want %q", auth.AccountID, "cli-acct")
	}
}

func TestLoadFailures(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := load("/nonexistent/auth.json")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("empty access token", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "auth.json")
		os.WriteFile(path, []byte(`{"tokens":{"access_token":"","refresh_token":"r"}}`), 0o600)

		_, err := load(path)
		if err == nil {
			t.Fatal("expected error for empty access_token")
		}
	})
}

func TestTokenReturnsCachedWhenFresh(t *testing.T) {
	auth := &Auth{
		accessToken: "fresh-token",
		expiresAt:   time.Now().Add(time.Hour),
	}

	tok, err := auth.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "fresh-token" {
		t.Errorf("Token() = %q, want %q", tok, "fresh-token")
	}
}

func TestTokenFailsWithNoRefreshToken(t *testing.T) {
	auth := &Auth{
		accessToken: "expired-token",
		expiresAt:   time.Now().Add(-time.Hour),
	}

	_, err := auth.Token()
	if err == nil {
		t.Fatal("expected error when token expired and no refresh token")
	}
}

func TestSave(t *testing.T) {
	t.Run("creates directory", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "deep", "auth.json")

		auth := &Auth{
			AccountID:    "acct",
			accessToken:  "tok",
			refreshToken: "ref",
			expiresAt:    time.Now().Add(time.Hour),
			filePath:     path,
		}

		err := auth.save()
		if err != nil {
			t.Fatalf("save: %v", err)
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Fatal("expected file to be created")
		}
	})

	t.Run("file permissions", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "auth.json")

		auth := &Auth{
			accessToken: "tok",
			expiresAt:   time.Now().Add(time.Hour),
			filePath:    path,
		}

		err := auth.save()
		if err != nil {
			t.Fatalf("save: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		perm := info.Mode().Perm()
		if perm != 0o600 {
			t.Errorf("file permissions = %o, want 600", perm)
		}
	})
}

// SignedIn asks whether there is anything to load, and nothing more.
//
// It is called from the daemon's not-ready gate, which re-runs every couple of
// seconds — so it must never reach the provider. A version that validated
// would be a token refresh on a timer for as long as a nik waits to be set up.
func TestSignedInIsExistenceAndNotValidity(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if SignedIn() {
		t.Error("reports a sign-in with nothing on disk")
	}

	// Garbage, deliberately: existence is the question, and a file that
	// cannot be parsed is Load's problem rather than this one's.
	if err := os.MkdirAll(filepath.Join(dir, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".codex", "auth.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !SignedIn() {
		t.Error("does not see a sign-in that is sitting right there")
	}
	// And the broken one is still refused where refusing it means something.
	if _, err := Load(""); err == nil {
		t.Error("Load accepted a file that is not a sign-in")
	}
}
