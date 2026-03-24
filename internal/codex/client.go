package codex

import (
	"fmt"
	"net/http"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func BuildOpenAIClient(apiKey string) (*openai.Client, error) {
	if apiKey != "" {
		c := openai.NewClient(option.WithAPIKey(apiKey))
		return &c, nil
	}

	auth, err := LoadOrLogin("")
	if err != nil {
		return nil, fmt.Errorf("codex auth: %w", err)
	}

	mw := func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		token, err := auth.Token()
		if err != nil {
			return nil, fmt.Errorf("codex token refresh: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+token)
		if auth.AccountID != "" {
			req.Header.Set("Chatgpt-Account-Id", auth.AccountID)
		}

		return next(req)
	}

	c := openai.NewClient(
		option.WithAPIKey("codex-oauth"),
		option.WithBaseURL("https://chatgpt.com/backend-api/codex"),
		option.WithMiddleware(mw),
		option.WithHeader("originator", "codex_cli_rs"),
	)

	return &c, nil
}
