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

func TestLoadMissingFile(t *testing.T) {
	_, err := load("/nonexistent/auth.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadEmptyAccessToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	err := os.WriteFile(path, []byte(`{"tokens":{"access_token":"","refresh_token":"r"}}`), 0o600)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err = load(path)
	if err == nil {
		t.Fatal("expected error for empty access_token")
	}
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

func TestSaveCreatesDirectory(t *testing.T) {
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
}

func TestSaveFilePermissions(t *testing.T) {
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
}
