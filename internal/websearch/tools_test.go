package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
)

func TestBuildTools_disabled(t *testing.T) {
	cfg := &config.Config{}
	tools := BuildTools(cfg)
	if tools != nil {
		t.Fatalf("expected nil tools when exa_api_key is empty, got %d", len(tools))
	}
}

func TestBuildTools_enabled(t *testing.T) {
	cfg := &config.Config{ExaAPIKey: "test-key"}
	tools := BuildTools(cfg)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Def.Name != "web_search" {
		t.Fatalf("expected tool name web_search, got %s", tools[0].Def.Name)
	}
}

func TestHandler_emptyQuery(t *testing.T) {
	handler := webSearchHandler("test-key")

	call := llm.ToolCall{
		CallID:    "test",
		Name:      "web_search",
		Arguments: `{"query":"","num_results":5,"category":""}`,
	}

	result, err := handler(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != `{"error":"empty query"}` {
		t.Fatalf("expected empty query error, got %s", result)
	}
}

func TestHandler_invalidJSON(t *testing.T) {
	handler := webSearchHandler("test-key")

	call := llm.ToolCall{
		CallID:    "test",
		Name:      "web_search",
		Arguments: `{bad json`,
	}

	result, err := handler(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]string
	err = json.Unmarshal([]byte(result), &parsed)
	if err != nil {
		t.Fatalf("result is not valid json: %s", result)
	}
	if _, ok := parsed["error"]; !ok {
		t.Fatalf("expected error field in result: %s", result)
	}
}

func TestHandler_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/search" {
			t.Errorf("expected /search, got %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key test-key, got %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		var req exaRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Query != "golang testing" {
			t.Errorf("expected query 'golang testing', got %q", req.Query)
		}
		if req.NumResults != 3 {
			t.Errorf("expected numResults 3, got %d", req.NumResults)
		}
		if req.Type != "auto" {
			t.Errorf("expected type auto, got %s", req.Type)
		}
		if req.Category != "news" {
			t.Errorf("expected category news, got %s", req.Category)
		}

		resp := exaResponse{
			Results: []exaResult{
				{Title: "Go Testing", URL: "https://example.com/go", Text: "Testing in Go is great."},
				{Title: "Unit Tests", URL: "https://example.com/unit", Text: "Unit testing best practices."},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	old := baseURL
	baseURL = srv.URL
	defer func() { baseURL = old }()

	handler := webSearchHandler("test-key")

	call := llm.ToolCall{
		CallID:    "test",
		Name:      "web_search",
		Arguments: `{"query":"golang testing","num_results":3,"category":"news"}`,
	}

	result, err := handler(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Results []resultEntry `json:"results"`
	}
	err = json.Unmarshal([]byte(result), &parsed)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}

	if len(parsed.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(parsed.Results))
	}
	if parsed.Results[0].Title != "Go Testing" {
		t.Errorf("expected title 'Go Testing', got %q", parsed.Results[0].Title)
	}
	if parsed.Results[1].URL != "https://example.com/unit" {
		t.Errorf("expected url 'https://example.com/unit', got %q", parsed.Results[1].URL)
	}
}

func TestHandler_apiError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	old := baseURL
	baseURL = srv.URL
	defer func() { baseURL = old }()

	handler := webSearchHandler("bad-key")

	call := llm.ToolCall{
		CallID:    "test",
		Name:      "web_search",
		Arguments: `{"query":"test","num_results":5,"category":""}`,
	}

	result, err := handler(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]string
	err = json.Unmarshal([]byte(result), &parsed)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if _, ok := parsed["error"]; !ok {
		t.Fatalf("expected error field in result: %s", result)
	}
}

func TestHandler_defaultNumResults(t *testing.T) {
	var capturedReq exaRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(exaResponse{Results: []exaResult{}})
	}))
	defer srv.Close()

	old := baseURL
	baseURL = srv.URL
	defer func() { baseURL = old }()

	handler := webSearchHandler("test-key")

	call := llm.ToolCall{
		CallID:    "test",
		Name:      "web_search",
		Arguments: `{"query":"test","num_results":0,"category":""}`,
	}

	_, err := handler(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq.NumResults != 5 {
		t.Errorf("expected default numResults 5, got %d", capturedReq.NumResults)
	}
}

func TestHandler_clampNumResults(t *testing.T) {
	var capturedReq exaRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(exaResponse{Results: []exaResult{}})
	}))
	defer srv.Close()

	old := baseURL
	baseURL = srv.URL
	defer func() { baseURL = old }()

	handler := webSearchHandler("test-key")

	call := llm.ToolCall{
		CallID:    "test",
		Name:      "web_search",
		Arguments: `{"query":"test","num_results":50,"category":""}`,
	}

	_, err := handler(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq.NumResults != 20 {
		t.Errorf("expected clamped numResults 20, got %d", capturedReq.NumResults)
	}
}
