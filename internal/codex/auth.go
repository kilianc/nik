package codex

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	issuer = "https://auth.openai.com"
	// public OAuth client ID for the Codex CLI "sign in with OpenAI" flow
	clientID      = "app_EMoamEEZ73f0CkXaXp7hrann"
	tokenEndpoint = "https://auth.openai.com/oauth/token"
	scopes        = "openid profile email offline_access"
	refreshBuffer = 5 * time.Minute
)

type Auth struct {
	AccountID string

	mu           sync.Mutex
	accessToken  string
	refreshToken string
	expiresAt    time.Time
	filePath     string
}

type authFile struct {
	Tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

func Load(path string) (*Auth, error) {
	path, err := resolvePath(path)
	if err != nil {
		return nil, err
	}

	auth, err := load(path)
	if err != nil {
		return nil, err
	}

	_, err = auth.Token()
	if err != nil {
		return nil, fmt.Errorf("validate token: %w", err)
	}

	return auth, nil
}

func LoadOrLogin(path string) (*Auth, error) {
	path, err := resolvePath(path)
	if err != nil {
		return nil, err
	}

	auth, err := load(path)
	if err != nil {
		slog.Info("codex auth not found, starting login", "pkg", "codex", "error", err)
		return login(path)
	}

	_, err = auth.Token()
	if err != nil {
		slog.Info("codex token refresh failed, re-authenticating", "pkg", "codex", "error", err)
		return login(path)
	}

	return auth, nil
}

func resolvePath(path string) (string, error) {
	if path != "" {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	return filepath.Join(home, ".codex", "auth.json"), nil
}

func load(path string) (*Auth, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var af authFile
	err = json.Unmarshal(data, &af)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if af.Tokens.AccessToken == "" {
		return nil, fmt.Errorf("no access_token in %s", path)
	}

	var expiresAt time.Time
	if af.ExpiresAt != "" {
		expiresAt, _ = time.Parse(time.RFC3339, af.ExpiresAt)
	}

	if expiresAt.IsZero() {
		stat, err := os.Stat(path)
		if err == nil {
			expiresAt = stat.ModTime().Add(time.Hour)
		} else {
			expiresAt = time.Now().Add(time.Hour)
		}
	}

	return &Auth{
		AccountID:    af.Tokens.AccountID,
		accessToken:  af.Tokens.AccessToken,
		refreshToken: af.Tokens.RefreshToken,
		expiresAt:    expiresAt,
		filePath:     path,
	}, nil
}

func (a *Auth) Token() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if time.Now().Before(a.expiresAt.Add(-refreshBuffer)) {
		return a.accessToken, nil
	}

	err := a.refresh()
	if err != nil {
		return "", err
	}

	return a.accessToken, nil
}

func (a *Auth) refresh() error {
	if a.refreshToken == "" {
		return fmt.Errorf("no refresh token")
	}

	data := url.Values{
		"client_id":     {clientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {a.refreshToken},
	}

	resp, err := http.PostForm(tokenEndpoint, data)
	if err != nil {
		return fmt.Errorf("refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		excerpt := string(body)
		if len(excerpt) > 200 {
			excerpt = excerpt[:200] + "…"
		}
		return fmt.Errorf("refresh token (HTTP %d): %s", resp.StatusCode, excerpt)
	}

	var tokenResp tokenResponse
	err = json.Unmarshal(body, &tokenResp)
	if err != nil {
		return fmt.Errorf("parse refresh response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("no access_token in refresh response")
	}

	a.accessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		a.refreshToken = tokenResp.RefreshToken
	}

	if tokenResp.ExpiresIn > 0 {
		a.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	} else {
		a.expiresAt = time.Now().Add(time.Hour)
	}

	if id := extractAccountID(tokenResp.AccessToken); id != "" {
		a.AccountID = id
	} else if id := extractAccountID(tokenResp.IDToken); id != "" {
		a.AccountID = id
	}

	slog.Info("codex token refreshed", "pkg", "codex", "expires_at", a.expiresAt.Format(time.RFC3339))

	return a.save()
}

func (a *Auth) save() error {
	dir := filepath.Dir(a.filePath)
	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	af := authFile{
		ExpiresAt: a.expiresAt.Format(time.RFC3339),
	}
	af.Tokens.AccessToken = a.accessToken
	af.Tokens.RefreshToken = a.refreshToken
	af.Tokens.AccountID = a.AccountID

	data, err := json.MarshalIndent(af, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth: %w", err)
	}

	err = os.WriteFile(a.filePath, data, 0o600)
	if err != nil {
		return fmt.Errorf("write %s: %w", a.filePath, err)
	}

	return nil
}
