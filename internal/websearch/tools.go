package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
)

const defaultBaseURL = "https://api.exa.ai"

var webSearchToolDef = llm.ToolDef{
	Name:        "web_search",
	Description: "Search the web for current information. Accepts multiple queries at once to reduce round-trips. Returns titles, URLs, and text highlights keyed by query.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"queries": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "One or more search queries. Can be questions, topics, or descriptive phrases.",
			},
			"num_results": map[string]any{
				"type":        "integer",
				"description": "Number of results per query (1-20). Default 5.",
			},
			"category": map[string]any{
				"type":        "string",
				"enum":        []string{"news", "research paper", "tweet", "company", "people"},
				"description": "Optional category to focus on.",
			},
		},
		"required":             []string{"queries", "num_results", "category"},
		"additionalProperties": false,
	},
}

type searchArgs struct {
	Queries    []string `json:"queries"`
	NumResults int      `json:"num_results"`
	Category   string   `json:"category"`
}

type exaRequest struct {
	Query      string      `json:"query"`
	Type       string      `json:"type"`
	NumResults int         `json:"numResults"`
	Category   string      `json:"category,omitempty"`
	Contents   exaContents `json:"contents"`
}

type exaContents struct {
	Text exaText `json:"text"`
}

type exaText struct {
	MaxCharacters int `json:"maxCharacters"`
}

type exaResponse struct {
	Results []exaResult `json:"results"`
}

type exaResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Text  string `json:"text"`
}

type resultEntry struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Text  string `json:"text"`
}

// baseURL is overridden in tests.
var baseURL = defaultBaseURL

func BuildTools(cfg *config.Config) []llm.Tool {
	if cfg.ExaAPIKey == "" {
		slog.Warn("web_search tool disabled: missing exa_api_key", "pkg", "websearch")
		return nil
	}

	return []llm.Tool{
		{
			Def:     webSearchToolDef,
			Handler: webSearchHandler(cfg.ExaAPIKey),
		},
	}
}

func webSearchHandler(apiKey string) llm.ToolExecutor {
	client := &http.Client{Timeout: 30 * time.Second}

	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args searchArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if len(args.Queries) == 0 {
			return `{"error":"empty queries"}`, nil
		}

		numResults := args.NumResults
		if numResults <= 0 {
			numResults = 5
		}
		if numResults > 20 {
			numResults = 20
		}

		results := map[string]any{}

		for _, q := range args.Queries {
			result, err := DoSearch(ctx, client, apiKey, q, numResults, args.Category)
			if err != nil {
				results[q] = map[string]any{"error": err.Error()}
				continue
			}

			var parsed any
			_ = json.Unmarshal([]byte(result), &parsed)
			results[q] = parsed
		}

		b, _ := json.Marshal(map[string]any{"results": results})
		return string(b), nil
	}
}

func DoSearch(ctx context.Context, client *http.Client, apiKey, query string, numResults int, category string) (string, error) {
	reqBody := exaRequest{
		Query:      query,
		Type:       "auto",
		NumResults: numResults,
		Category:   category,
		Contents: exaContents{
			Text: exaText{MaxCharacters: 4000},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/search", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("exa search: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("exa search %d: %s", resp.StatusCode, string(respBody))
	}

	var exaResp exaResponse
	err = json.Unmarshal(respBody, &exaResp)
	if err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	entries := make([]resultEntry, len(exaResp.Results))
	for i, r := range exaResp.Results {
		entries[i] = resultEntry{
			Title: r.Title,
			URL:   r.URL,
			Text:  r.Text,
		}
	}

	out, err := json.Marshal(map[string]any{"results": entries})
	if err != nil {
		return "", fmt.Errorf("marshal results: %w", err)
	}

	return string(out), nil
}
