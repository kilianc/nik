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
	activationFlag := flag.String("activation", "", "activation short ID to replay (default: diagnosed activation)")
	round := flag.Int("round", -1, "round to replay (-1 = use diagnosed round from case.json)")
	desired := flag.String("desired", "", "desired tool pattern (e.g. message_noop, message_send+task_spawn)")
	effortOverride := flag.String("effort", "", "override reasoning effort (low, medium, high)")
	n := flag.Int("n", 1, "number of replay attempts")
	verbose := flag.Bool("v", false, "show full tool call arguments and per-attempt reasoning")
	jsonOut := flag.Bool("json", false, "output structured JSON (for programmatic use)")
	flag.Parse()

	if *caseDir == "" {
		fmt.Fprintln(os.Stderr, "usage: replay -case <path> [-activation ID] [-round N] [-desired pattern] [-n 1] [-v]")
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
	if targetRound < 0 && *activationFlag == "" {
		targetRound = cj.Diagnosis.Round
	}
	if targetRound < 0 {
		targetRound = 0
	}

	act := cj.Activations[0]
	if *activationFlag != "" {
		found := false
		for _, a := range cj.Activations {
			if strings.HasSuffix(a.ID, *activationFlag) {
				act = a
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "activation %q not found in case\n", *activationFlag)
			os.Exit(1)
		}
	} else if cj.Diagnosis.ActivationID != "" {
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

	items, err := buildItems(*caseDir, act, targetRound)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build items: %v\n", err)
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

	cfg := replayConfig{
		model:           act.Model,
		reasoningEffort: act.ReasoningEffort,
		verbosity:       act.Verbosity,
	}
	if *effortOverride != "" {
		cfg.reasoningEffort = *effortOverride
	}

	priorRounds := 0
	priorToolCalls := 0
	for _, r := range act.Rounds {
		if r.Round >= targetRound {
			break
		}
		priorRounds++
		priorToolCalls += len(r.ToolCalls)
	}

	if !*jsonOut {
		configLabel := ""
		if cfg.reasoningEffort != "" {
			configLabel += ", effort=" + cfg.reasoningEffort
		}
		if cfg.verbosity != "" {
			configLabel += ", verbosity=" + cfg.verbosity
		}
		fmt.Printf("REPLAY  activation %s  round %d  (%s%s)\n", shortID(act.ID), targetRound, act.Model, configLabel)
		fmt.Printf("  instructions: %d chars\n", len(instructions))
		fmt.Printf("  tools:        %d tools\n", len(tools))
		fmt.Printf("  items:        %d (1 user msg + %d prior rounds, %d tool call pairs)\n",
			len(items), priorRounds, priorToolCalls)
		if act.InputTokens > 0 {
			fmt.Printf("  original:     in=%d out=%d cached=%d reasoning=%d\n",
				act.InputTokens, act.OutputTokens, act.CachedTokens, act.ReasoningTokens)
		}
		fmt.Println()
	}

	var attempts []replayResult

	for i := range *n {
		result, err := replayOnce(context.Background(), client, string(instructions), items, tools, cfg, *verbose || *jsonOut)
		if err != nil {
			fmt.Fprintf(os.Stderr, "attempt %d failed: %v\n", i+1, err)
			continue
		}

		attempts = append(attempts, result)

		if *jsonOut {
			continue
		}

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

		showReasoning := *n == 1 || *verbose
		if showReasoning && result.reasoning != "" {
			limit := 200
			if *verbose {
				limit = 2000
			}
			fmt.Println("  reasoning:")
			fmt.Printf("    %s\n", truncate(strings.TrimSpace(result.reasoning), limit))
		}

		if *verbose {
			fmt.Printf("  tokens: in=%d out=%d cached=%d reasoning=%d\n",
				result.inputTokens, result.outputTokens, result.cachedTokens, result.reasoningTokens)
		}
	}

	origKey := originalKey(roundData)

	if *jsonOut {
		printJSON(act, targetRound, roundData, attempts, origKey, *desired)
		return
	}

	fmt.Println()
	fmt.Printf("ORIGINAL: %s\n", origKey)
	if *desired != "" {
		fmt.Printf("DESIRED:  %s\n", *desired)
	}

	if *n > 1 && len(attempts) > 0 {
		fmt.Println()
		printDistribution(attempts, origKey, *desired)
	}
}

func originalKey(roundData *caseRound) string {
	if len(roundData.ToolCalls) == 0 {
		return "no_tools"
	}
	var names []string
	for _, tc := range roundData.ToolCalls {
		names = append(names, tc.Name)
	}
	return strings.Join(names, "+")
}

func attemptKey(a replayResult) string {
	if len(a.tools) == 0 {
		return "no_tools"
	}
	var names []string
	for _, tc := range a.tools {
		name, _, _ := strings.Cut(tc, "  ")
		names = append(names, name)
	}
	return strings.Join(names, "+")
}

func printDistribution(attempts []replayResult, origKey, desiredKey string) {
	counts := map[string]int{}
	for _, a := range attempts {
		counts[attemptKey(a)]++
	}

	fmt.Println("DISTRIBUTION:")
	for k, v := range counts {
		var labels []string
		if k == origKey {
			labels = append(labels, "original")
		}
		if desiredKey != "" && k == desiredKey {
			labels = append(labels, "desired")
		}
		tag := ""
		if len(labels) > 0 {
			tag = " <- " + strings.Join(labels, ", ")
		}
		fmt.Printf("  %s: %d/%d (%.0f%%)%s\n", k, v, len(attempts), float64(v)/float64(len(attempts))*100, tag)
	}
}

type jsonOutput struct {
	ActivationID    string          `json:"activation_id"`
	Round           int             `json:"round"`
	Model           string          `json:"model"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	Verbosity       string          `json:"verbosity,omitempty"`
	OriginalKey     string          `json:"original_key"`
	DesiredKey      string          `json:"desired_key,omitempty"`
	OriginalTools   []string        `json:"original_tools"`
	Attempts        []jsonAttempt   `json:"attempts"`
	Distribution    []jsonDistEntry `json:"distribution"`
}

type jsonDistEntry struct {
	Key        string `json:"key"`
	Count      int    `json:"count"`
	Percent    int    `json:"percent"`
	IsOriginal bool   `json:"is_original"`
	IsDesired  bool   `json:"is_desired"`
}

type jsonAttempt struct {
	Tools           []jsonToolCall `json:"tools"`
	Reasoning       string         `json:"reasoning,omitempty"`
	Text            string         `json:"text,omitempty"`
	InputTokens     int            `json:"input_tokens"`
	OutputTokens    int            `json:"output_tokens"`
	CachedTokens    int            `json:"cached_tokens"`
	ReasoningTokens int            `json:"reasoning_tokens"`
}

type jsonToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func printJSON(act caseActivation, targetRound int, roundData *caseRound, attempts []replayResult, origKey, desiredKey string) {
	var origTools []string
	for _, tc := range roundData.ToolCalls {
		origTools = append(origTools, tc.Name)
	}

	counts := map[string]int{}
	var jAttempts []jsonAttempt

	for _, a := range attempts {
		var tcs []jsonToolCall
		key := attemptKey(a)
		if len(a.tools) > 0 {
			for _, tc := range a.tools {
				name, args, _ := strings.Cut(tc, "  ")
				tcs = append(tcs, jsonToolCall{Name: name, Arguments: args})
			}
		}
		counts[key]++

		jAttempts = append(jAttempts, jsonAttempt{
			Tools:           tcs,
			Reasoning:       strings.TrimSpace(a.reasoning),
			Text:            a.text,
			InputTokens:     a.inputTokens,
			OutputTokens:    a.outputTokens,
			CachedTokens:    a.cachedTokens,
			ReasoningTokens: a.reasoningTokens,
		})
	}

	total := len(attempts)
	var dist []jsonDistEntry
	for k, v := range counts {
		pct := 0
		if total > 0 {
			pct = v * 100 / total
		}
		dist = append(dist, jsonDistEntry{
			Key:        k,
			Count:      v,
			Percent:    pct,
			IsOriginal: k == origKey,
			IsDesired:  desiredKey != "" && k == desiredKey,
		})
	}

	out := jsonOutput{
		ActivationID:    act.ID,
		Round:           targetRound,
		Model:           act.Model,
		ReasoningEffort: act.ReasoningEffort,
		Verbosity:       act.Verbosity,
		OriginalKey:     origKey,
		DesiredKey:      desiredKey,
		OriginalTools:   origTools,
		Attempts:        jAttempts,
		Distribution:    dist,
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(data))
}

type caseJSON struct {
	Message      json.RawMessage  `json:"message"`
	Conversation json.RawMessage  `json:"conversation"`
	Activations  []caseActivation `json:"activations"`
	Diagnosis    caseDiagnosis    `json:"diagnosis"`
}

type caseActivation struct {
	ID              string      `json:"id"`
	Model           string      `json:"model"`
	ReasoningEffort string      `json:"reasoning_effort,omitempty"`
	Verbosity       string      `json:"verbosity,omitempty"`
	Sources         []string    `json:"sources,omitempty"`
	Tools           []string    `json:"tools"`
	CreatedAt       string      `json:"created_at"`
	DurationMS      int         `json:"duration_ms"`
	InputTokens     int         `json:"input_tokens"`
	OutputTokens    int         `json:"output_tokens"`
	CachedTokens    int         `json:"cached_tokens"`
	ReasoningTokens int         `json:"reasoning_tokens"`
	Error           string      `json:"error,omitempty"`
	Rounds          []caseRound `json:"rounds"`
}

type caseRound struct {
	Round              int            `json:"round"`
	InputFile          string         `json:"input_file"`
	OutputFile         string         `json:"output_file,omitempty"`
	MessagePresent     bool           `json:"message_present"`
	MessageInNew       bool           `json:"message_in_new"`
	ReasoningSummaries []string       `json:"reasoning_summaries,omitempty"`
	ToolCalls          []caseToolCall `json:"tool_calls"`
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
	tools           []string
	reasoning       string
	text            string
	inputTokens     int
	outputTokens    int
	cachedTokens    int
	reasoningTokens int
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

type replayConfig struct {
	model           string
	reasoningEffort string
	verbosity       string
}

func buildItems(caseDir string, act caseActivation, targetRound int) (responses.ResponseInputParam, error) {
	var roundData *caseRound
	for i := range act.Rounds {
		if act.Rounds[i].Round == targetRound {
			roundData = &act.Rounds[i]
			break
		}
	}
	if roundData == nil {
		return nil, fmt.Errorf("round %d not found", targetRound)
	}

	userInput, err := os.ReadFile(filepath.Join(caseDir, roundData.InputFile))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", roundData.InputFile, err)
	}

	items := responses.ResponseInputParam{
		responses.ResponseInputItemParamOfMessage(string(userInput), responses.EasyInputMessageRoleUser),
	}

	callSeq := 0
	for _, r := range act.Rounds {
		if r.Round >= targetRound {
			break
		}

		if r.OutputFile != "" {
			modelOutput, readErr := os.ReadFile(filepath.Join(caseDir, r.OutputFile))
			if readErr == nil && len(modelOutput) > 0 {
				items = append(items, responses.ResponseInputItemParamOfMessage(string(modelOutput), responses.EasyInputMessageRoleAssistant))
			}
		}

		for _, tc := range r.ToolCalls {
			callID := fmt.Sprintf("call_replay_%d", callSeq)
			callSeq++
			items = append(items, responses.ResponseInputItemParamOfFunctionCall(tc.Input, callID, tc.Name))
			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(callID, tc.Output))
		}
	}

	return items, nil
}

func replayOnce(ctx context.Context, client *openai.Client, instructions string, items responses.ResponseInputParam, tools []toolSchema, cfg replayConfig, fullArgs bool) (replayResult, error) {
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
		Model:        shared.ResponsesModel(cfg.model),
		Instructions: openai.String(instructions),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: items,
		},
		Tools: toolParams,
		Store: openai.Bool(false),
		Reasoning: shared.ReasoningParam{
			Summary: shared.ReasoningSummaryDetailed,
		},
	}

	if cfg.reasoningEffort != "" {
		params.Reasoning.Effort = shared.ReasoningEffort(cfg.reasoningEffort)
	}

	if cfg.verbosity != "" {
		params.Text = responses.ResponseTextConfigParam{
			Verbosity: responses.ResponseTextConfigVerbosity(cfg.verbosity),
		}
	}

	resp, err := completeCall(ctx, client, params)
	if err != nil {
		return replayResult{}, fmt.Errorf("api call: %w", err)
	}

	var result replayResult
	result.text = resp.OutputText()
	result.inputTokens = int(resp.Usage.InputTokens)
	result.outputTokens = int(resp.Usage.OutputTokens)
	result.cachedTokens = int(resp.Usage.InputTokensDetails.CachedTokens)
	result.reasoningTokens = int(resp.Usage.OutputTokensDetails.ReasoningTokens)

	argLimit := 100
	if fullArgs {
		argLimit = 2000
	}

	for _, item := range resp.Output {
		if item.Type == "function_call" {
			fc := item.AsFunctionCall()
			result.tools = append(result.tools, fmt.Sprintf("%s  %s", fc.Name, truncate(fc.Arguments, argLimit)))
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
