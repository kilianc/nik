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

func TestHandler_emptyQueries(t *testing.T) {
	handler := webSearchHandler("test-key")

	call := llm.ToolCall{
		CallID:    "test",
		Name:      "web_search",
		Arguments: `{"queries":[],"num_results":5,"category":""}`,
	}

	result, err := handler(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != `{"error":"empty queries"}` {
		t.Fatalf("expected empty queries error, got %s", result)
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

		var req exaRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Type != "auto" {
			t.Errorf("expected type auto, got %s", req.Type)
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
		Arguments: `{"queries":["golang testing"],"num_results":3,"category":"news"}`,
	}

	result, err := handler(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Results map[string]struct {
			Results []resultEntry `json:"results"`
		} `json:"results"`
	}
	err = json.Unmarshal([]byte(result), &parsed)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}

	qr, ok := parsed.Results["golang testing"]
	if !ok {
		t.Fatalf("expected results for 'golang testing', keys: %v", parsed.Results)
	}

	if len(qr.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(qr.Results))
	}
	if qr.Results[0].Title != "Go Testing" {
		t.Errorf("expected title 'Go Testing', got %q", qr.Results[0].Title)
	}
	if qr.Results[1].URL != "https://example.com/unit" {
		t.Errorf("expected url 'https://example.com/unit', got %q", qr.Results[1].URL)
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
		Arguments: `{"queries":["test"],"num_results":5,"category":""}`,
	}

	result, err := handler(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Results map[string]map[string]string `json:"results"`
	}
	err = json.Unmarshal([]byte(result), &parsed)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	qr, ok := parsed.Results["test"]
	if !ok {
		t.Fatalf("expected results for 'test': %s", result)
	}
	if _, ok := qr["error"]; !ok {
		t.Fatalf("expected error field for query 'test': %s", result)
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
		Arguments: `{"queries":["test"],"num_results":0,"category":""}`,
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
		Arguments: `{"queries":["test"],"num_results":50,"category":""}`,
	}

	_, err := handler(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq.NumResults != 20 {
		t.Errorf("expected clamped numResults 20, got %d", capturedReq.NumResults)
	}
}

func TestHandler_multipleQueries(t *testing.T) {
	var queries []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req exaRequest
		json.NewDecoder(r.Body).Decode(&req)
		queries = append(queries, req.Query)

		resp := exaResponse{
			Results: []exaResult{
				{Title: req.Query + " result", URL: "https://example.com/" + req.Query, Text: "text"},
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
		Arguments: `{"queries":["weather NYC","flights Boston"],"num_results":3,"category":""}`,
	}

	result, err := handler(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(queries) != 2 {
		t.Fatalf("expected 2 API calls, got %d", len(queries))
	}

	var parsed struct {
		Results map[string]json.RawMessage `json:"results"`
	}
	err = json.Unmarshal([]byte(result), &parsed)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}

	if _, ok := parsed.Results["weather NYC"]; !ok {
		t.Errorf("missing results for 'weather NYC'")
	}
	if _, ok := parsed.Results["flights Boston"]; !ok {
		t.Errorf("missing results for 'flights Boston'")
	}
}
