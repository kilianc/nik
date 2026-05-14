package codex

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"testing"
)

func TestExtractAccountID(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
		raw    string
		want   string
	}{
		{
			"direct claim",
			map[string]any{"chatgpt_account_id": "acct-direct"},
			"",
			"acct-direct",
		},
		{
			"nested claim",
			map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acct-nested"}},
			"",
			"acct-nested",
		},
		{
			"organizations",
			map[string]any{"organizations": []any{map[string]any{"id": "org-123"}}},
			"",
			"org-123",
		},
		{"invalid token", nil, "not-a-jwt", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := tt.raw
			if tt.claims != nil {
				token = buildTestJWT(tt.claims)
			}
			got := extractAccountID(token)
			if got != tt.want {
				t.Errorf("extractAccountID = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractCode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		state string
		want  string
	}{
		{"valid url", "http://localhost:1455/auth/callback?code=abc123&state=test-state", "test-state", "abc123"},
		{"state mismatch", "http://localhost:1455/auth/callback?code=abc123&state=wrong-state", "expected-state", ""},
		{"bare input", "just-a-code", "state", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCode(tt.input, tt.state)
			if got != tt.want {
				t.Errorf("extractCode = %q, want %q", got, tt.want)
			}
		})
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

func TestPrepareLogin(t *testing.T) {
	req, err := PrepareLogin()
	if err != nil {
		t.Fatalf("PrepareLogin: %v", err)
	}

	u, err := url.Parse(req.AuthURL)
	if err != nil {
		t.Fatalf("parse AuthURL: %v", err)
	}
	if u.Host != "auth.openai.com" {
		t.Errorf("host = %q, want auth.openai.com", u.Host)
	}
	q := u.Query()
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q, want code", q.Get("response_type"))
	}
	if q.Get("client_id") != clientID {
		t.Errorf("client_id = %q, want %q", q.Get("client_id"), clientID)
	}
	if q.Get("redirect_uri") != redirectURI {
		t.Errorf("redirect_uri = %q, want %q", q.Get("redirect_uri"), redirectURI)
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", q.Get("code_challenge_method"))
	}
	if q.Get("state") != req.state {
		t.Error("state in URL doesn't match request state")
	}
	if req.verifier == "" {
		t.Error("verifier empty")
	}
}

func TestCompleteRejectsEmptyInput(t *testing.T) {
	req := &AuthRequest{verifier: "v", state: "s"}
	_, err := req.Complete("   ", t.TempDir()+"/auth.json")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func buildTestJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(claims)
	body := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + body + ".sig"
}
