package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kciuffolo/nik/internal/codex"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

func main() {
	caseDir := flag.String("case", "", "path to case directory")
	round := flag.Int("round", -1, "round to replay (-1 = use diagnosed round from case.json)")
	n := flag.Int("n", 1, "number of replay attempts")
	flag.Parse()

	if *caseDir == "" {
		fmt.Fprintln(os.Stderr, "usage: replay -case <path> [-round N] [-n 1]")
		os.Exit(1)
	}

	cj, err := loadCase(*caseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load case: %v\n", err)
		os.Exit(1)
	}

	if len(cj.Activations) == 0 {
		fmt.Fprintln(os.Stderr, "no activations in case")
		os.Exit(1)
	}

	targetRound := *round
	if targetRound < 0 {
		targetRound = cj.Diagnosis.Round
	}

	act := cj.Activations[0]
	if cj.Diagnosis.ActivationID != "" {
		for _, a := range cj.Activations {
			if a.ID == cj.Diagnosis.ActivationID {
				act = a
				break
			}
		}
	}

	var roundData *caseRound
	for i := range act.Rounds {
		if act.Rounds[i].Round == targetRound {
			roundData = &act.Rounds[i]
			break
		}
	}
	if roundData == nil {
		fmt.Fprintf(os.Stderr, "round %d not found in activation %s\n", targetRound, shortID(act.ID))
		os.Exit(1)
	}

	instructions, err := os.ReadFile(filepath.Join(*caseDir, "instructions.txt"))
	if err != nil {
		prefixed := fmt.Sprintf("%s_instructions.txt", shortID(act.ID))
		instructions, err = os.ReadFile(filepath.Join(*caseDir, prefixed))
		if err != nil {
			fmt.Fprintf(os.Stderr, "read instructions: %v\n", err)
			os.Exit(1)
		}
	}

	userInput, err := os.ReadFile(filepath.Join(*caseDir, roundData.InputFile))
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", roundData.InputFile, err)
		os.Exit(1)
	}

	tools, err := loadToolSchemas(*caseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load tools: %v\n", err)
		os.Exit(1)
	}

	client, err := buildClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build client: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("REPLAY  activation %s  round %d  (%s)\n", shortID(act.ID), targetRound, act.Model)
	fmt.Printf("  instructions: %d chars\n", len(instructions))
	fmt.Printf("  tools:        %d tools\n", len(tools))
	fmt.Printf("  input:        %d chars (%s)\n", len(userInput), roundData.InputFile)
	fmt.Println()

	var attempts []replayResult

	for i := range *n {
		result, err := replayOnce(context.Background(), client, string(instructions), string(userInput), tools, act.Model)
		if err != nil {
			fmt.Fprintf(os.Stderr, "attempt %d failed: %v\n", i+1, err)
			continue
		}

		attempts = append(attempts, result)

		if *n == 1 {
			fmt.Println("ATTEMPT 1/1")
		} else {
			fmt.Printf("  attempt %d: ", i+1)
		}

		if len(result.tools) == 0 {
			if *n == 1 {
				fmt.Println("  (no tool calls)")
			} else {
				fmt.Println("(no tool calls)")
			}
		} else {
			if *n == 1 {
				fmt.Println("  tool_calls:")
				for _, tc := range result.tools {
					fmt.Printf("    %s\n", tc)
				}
			} else {
				fmt.Println(strings.Join(result.tools, ", "))
			}
		}

		if *n == 1 && result.reasoning != "" {
			fmt.Println("  reasoning:")
			fmt.Printf("    %s\n", truncate(result.reasoning, 200))
		}
	}

	fmt.Println()
	fmt.Println("ORIGINAL (from case.json)")
	if len(roundData.ToolCalls) == 0 {
		fmt.Printf("  round %d: no tool calls\n", targetRound)
	} else {
		var names []string
		for _, tc := range roundData.ToolCalls {
			names = append(names, tc.Name)
		}
		fmt.Printf("  round %d: %s\n", targetRound, strings.Join(names, ", "))
	}

	if *n > 1 && len(attempts) > 0 {
		fmt.Println()
		counts := map[string]int{}
		for _, a := range attempts {
			key := "no_tools"
			if len(a.tools) > 0 {
				var names []string
				for _, tc := range a.tools {
					name, _, _ := strings.Cut(tc, "  ")
					names = append(names, name)
				}
				key = strings.Join(names, "+")
			}
			counts[key]++
		}

		fmt.Println("DISTRIBUTION:")
		for k, v := range counts {
			fmt.Printf("  %s: %d/%d (%.0f%%)\n", k, v, len(attempts), float64(v)/float64(len(attempts))*100)
		}
	}
}

type caseJSON struct {
	Message      json.RawMessage  `json:"message"`
	Conversation json.RawMessage  `json:"conversation"`
	Activations  []caseActivation `json:"activations"`
	Diagnosis    caseDiagnosis    `json:"diagnosis"`
}

type caseActivation struct {
	ID         string      `json:"id"`
	Model      string      `json:"model"`
	Tools      []string    `json:"tools"`
	CreatedAt  string      `json:"created_at"`
	DurationMS int         `json:"duration_ms"`
	Error      string      `json:"error,omitempty"`
	Rounds     []caseRound `json:"rounds"`
}

type caseRound struct {
	Round          int            `json:"round"`
	InputFile      string         `json:"input_file"`
	MessagePresent bool           `json:"message_present"`
	MessageInNew   bool           `json:"message_in_new"`
	ToolCalls      []caseToolCall `json:"tool_calls"`
}

type caseToolCall struct {
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output"`
	Error  int    `json:"error,omitempty"`
}

type caseDiagnosis struct {
	Category     string `json:"category"`
	Summary      string `json:"summary"`
	ActivationID string `json:"activation_id,omitempty"`
	Round        int    `json:"round,omitempty"`
}

type toolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func loadCase(dir string) (caseJSON, error) {
	data, err := os.ReadFile(filepath.Join(dir, "case.json"))
	if err != nil {
		return caseJSON{}, err
	}

	var cj caseJSON
	err = json.Unmarshal(data, &cj)
	return cj, err
}

func loadToolSchemas(dir string) ([]toolSchema, error) {
	data, err := os.ReadFile(filepath.Join(dir, "tools.json"))
	if err != nil {
		return nil, fmt.Errorf("read tools.json: %w", err)
	}

	var tools []toolSchema
	err = json.Unmarshal(data, &tools)
	if err != nil {
		return nil, fmt.Errorf("parse tools.json: %w", err)
	}

	return tools, nil
}

func buildClient() (*openai.Client, error) {
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		c := openai.NewClient(option.WithAPIKey(key))
		return &c, nil
	}

	return buildCodexClient()
}

func buildCodexClient() (*openai.Client, error) {
	auth, err := codex.LoadOrLogin("")
	if err != nil {
		return nil, fmt.Errorf("codex auth: %w", err)
	}

	mw := func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		token, tokenErr := auth.Token()
		if tokenErr != nil {
			return nil, fmt.Errorf("codex token refresh: %w", tokenErr)
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

type replayResult struct {
	tools     []string
	reasoning string
	text      string
}

func completeCall(ctx context.Context, client *openai.Client, params responses.ResponseNewParams) (*responses.Response, error) {
	stream := client.Responses.NewStreaming(ctx, params)
	defer stream.Close()

	var final *responses.Response
	for stream.Next() {
		evt := stream.Current()
		completed := evt.AsResponseCompleted()
		if completed.Type == "response.completed" {
			final = &completed.Response
		}
	}

	if stream.Err() != nil {
		return nil, stream.Err()
	}

	if final == nil {
		return nil, fmt.Errorf("stream ended without response.completed event")
	}

	return final, nil
}

func replayOnce(ctx context.Context, client *openai.Client, instructions, userInput string, tools []toolSchema, model string) (replayResult, error) {
	toolParams := make([]responses.ToolUnionParam, len(tools))
	for i, t := range tools {
		toolParams[i] = responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  t.Parameters,
				Strict:      openai.Bool(true),
			},
		}
	}

	params := responses.ResponseNewParams{
		Model:        shared.ResponsesModel(model),
		Instructions: openai.String(instructions),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				responses.ResponseInputItemParamOfMessage(userInput, responses.EasyInputMessageRoleUser),
			},
		},
		Tools: toolParams,
		Store: openai.Bool(false),
		Reasoning: shared.ReasoningParam{
			Summary: shared.ReasoningSummaryDetailed,
		},
	}

	resp, err := completeCall(ctx, client, params)
	if err != nil {
		return replayResult{}, fmt.Errorf("api call: %w", err)
	}

	var result replayResult
	result.text = resp.OutputText()

	for _, item := range resp.Output {
		if item.Type == "function_call" {
			fc := item.AsFunctionCall()
			result.tools = append(result.tools, fmt.Sprintf("%s  %s", fc.Name, truncate(fc.Arguments, 100)))
		}
		if item.Type == "reasoning" {
			for _, s := range item.AsReasoning().Summary {
				if s.Text != "" {
					result.reasoning += s.Text + "\n"
				}
			}
		}
	}

	return result, nil
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[len(id)-12:]
	}
	return id
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
