package codex

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
}

func login(savePath string) (*Auth, error) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, fmt.Errorf("generate PKCE: %w", err)
	}

	state, err := generateState()
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}

	redirectURI := "http://localhost:1455/auth/callback"

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {scopes},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}

	authURL := issuer + "/oauth/authorize?" + params.Encode()

	fmt.Println()
	fmt.Println("Codex login required. Open this URL in your browser:")
	fmt.Println()
	fmt.Println("  " + authURL)
	fmt.Println()
	fmt.Println("After signing in, paste the redirect URL here:")
	fmt.Print("> ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty input, login cancelled")
	}

	code := extractCode(input, state)
	if code == "" {
		code = input
	}

	auth, err := exchangeCode(code, verifier, redirectURI, savePath)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Logged in (account: %s)\n\n", auth.AccountID)

	return auth, nil
}

func extractCode(input, expectedState string) string {
	if !strings.Contains(input, "code=") && !strings.Contains(input, "?") {
		return ""
	}

	u, err := url.Parse(input)
	if err != nil {
		return ""
	}

	if s := u.Query().Get("state"); s != "" && s != expectedState {
		return ""
	}

	return u.Query().Get("code")
}

func exchangeCode(code, verifier, redirectURI, savePath string) (*Auth, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}

	resp, err := http.PostForm(tokenEndpoint, data)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		excerpt := string(body)
		if len(excerpt) > 200 {
			excerpt = excerpt[:200] + "…"
		}
		return nil, fmt.Errorf("token exchange (HTTP %d): %s", resp.StatusCode, excerpt)
	}

	var tokenResp tokenResponse
	err = json.Unmarshal(body, &tokenResp)
	if err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access_token in response")
	}

	var expiresAt time.Time
	if tokenResp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	} else {
		expiresAt = time.Now().Add(time.Hour)
	}

	accountID := extractAccountID(tokenResp.AccessToken)
	if accountID == "" {
		accountID = extractAccountID(tokenResp.IDToken)
	}

	auth := &Auth{
		AccountID:    accountID,
		accessToken:  tokenResp.AccessToken,
		refreshToken: tokenResp.RefreshToken,
		expiresAt:    expiresAt,
		filePath:     savePath,
	}

	err = auth.save()
	if err != nil {
		return nil, fmt.Errorf("save auth: %w", err)
	}

	return auth, nil
}

func extractAccountID(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}

	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	var claims map[string]any
	if json.Unmarshal(decoded, &claims) != nil {
		return ""
	}

	if id, ok := claims["chatgpt_account_id"].(string); ok && id != "" {
		return id
	}

	if authClaim, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		if id, ok := authClaim["chatgpt_account_id"].(string); ok && id != "" {
			return id
		}
	}

	if orgs, ok := claims["organizations"].([]any); ok {
		for _, org := range orgs {
			if orgMap, ok := org.(map[string]any); ok {
				if id, ok := orgMap["id"].(string); ok && id != "" {
					return id
				}
			}
		}
	}

	return ""
}

func generatePKCE() (verifier, challenge string, err error) {
	buf := make([]byte, 32)
	_, err = rand.Read(buf)
	if err != nil {
		return "", "", fmt.Errorf("random bytes: %w", err)
	}

	verifier = base64.RawURLEncoding.EncodeToString(buf)

	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])

	return verifier, challenge, nil
}

func generateState() (string, error) {
	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	if err != nil {
		return "", fmt.Errorf("random bytes: %w", err)
	}

	return hex.EncodeToString(buf), nil
}
