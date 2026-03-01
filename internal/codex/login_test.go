package codex

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestExtractAccountIDDirect(t *testing.T) {
	claims := map[string]any{
		"chatgpt_account_id": "acct-direct",
	}
	token := buildTestJWT(claims)

	id := extractAccountID(token)
	if id != "acct-direct" {
		t.Errorf("extractAccountID = %q, want %q", id, "acct-direct")
	}
}

func TestExtractAccountIDNested(t *testing.T) {
	claims := map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct-nested",
		},
	}
	token := buildTestJWT(claims)

	id := extractAccountID(token)
	if id != "acct-nested" {
		t.Errorf("extractAccountID = %q, want %q", id, "acct-nested")
	}
}

func TestExtractAccountIDOrganizations(t *testing.T) {
	claims := map[string]any{
		"organizations": []any{
			map[string]any{"id": "org-123"},
		},
	}
	token := buildTestJWT(claims)

	id := extractAccountID(token)
	if id != "org-123" {
		t.Errorf("extractAccountID = %q, want %q", id, "org-123")
	}
}

func TestExtractAccountIDInvalidToken(t *testing.T) {
	id := extractAccountID("not-a-jwt")
	if id != "" {
		t.Errorf("extractAccountID = %q, want empty", id)
	}
}

func TestExtractAccountIDEmptyToken(t *testing.T) {
	id := extractAccountID("")
	if id != "" {
		t.Errorf("extractAccountID = %q, want empty", id)
	}
}

func TestExtractCodeFromURL(t *testing.T) {
	state := "test-state"
	input := "http://localhost:1455/auth/callback?code=abc123&state=test-state"

	code := extractCode(input, state)
	if code != "abc123" {
		t.Errorf("extractCode = %q, want %q", code, "abc123")
	}
}

func TestExtractCodeStateMismatch(t *testing.T) {
	code := extractCode(
		"http://localhost:1455/auth/callback?code=abc123&state=wrong-state",
		"expected-state",
	)
	if code != "" {
		t.Errorf("extractCode = %q, want empty on state mismatch", code)
	}
}

func TestExtractCodeBareInput(t *testing.T) {
	code := extractCode("just-a-code", "state")
	if code != "" {
		t.Errorf("extractCode = %q, want empty for bare input", code)
	}
}

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		t.Fatalf("generatePKCE: %v", err)
	}

	if len(verifier) == 0 {
		t.Fatal("verifier is empty")
	}
	if len(challenge) == 0 {
		t.Fatal("challenge is empty")
	}

	h := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(h[:])
	if challenge != expected {
		t.Errorf("challenge mismatch: got %q, want %q", challenge, expected)
	}
}

func TestGenerateState(t *testing.T) {
	s1, err := generateState()
	if err != nil {
		t.Fatalf("generateState: %v", err)
	}

	s2, err := generateState()
	if err != nil {
		t.Fatalf("generateState: %v", err)
	}

	if len(s1) != 64 {
		t.Errorf("state length = %d, want 64 hex chars", len(s1))
	}

	if s1 == s2 {
		t.Error("two consecutive states should differ")
	}
}

func buildTestJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(claims)
	body := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + body + ".sig"
}
