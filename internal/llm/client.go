package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type CompletionObserver interface {
	OnStart(ctx context.Context, model string)
	OnToolCall(ctx context.Context, name string, args string, result string, duration time.Duration, isError bool)
	OnFinish(ctx context.Context, model string, reasoningEffort string, usage Usage, toolCalls int, durationMS int64, output string, processErr error)
	OnDetail(ctx context.Context, instructions string, userInput string, tools []string, reasoningSummaries []string)
}

type Completer interface {
	Complete(ctx context.Context, instructions string, getInput func() string, tools []ToolDef, executor ToolExecutor) (string, <-chan CompletionResult)
}

const maxConcurrentSessions = 6

type Client struct {
	codexClient     *openai.Client
	apiClient       *openai.Client
	model           *string
	reasoningEffort *string
	verbosity       *string
	observer        CompletionObserver
	sem             chan struct{}
}

func (c *Client) SetObserver(obs CompletionObserver) {
	c.observer = obs
}

type clientConfig struct {
	apiKey          string
	codexAuth       *codex.Auth
	reasoningEffort *string
	verbosity       *string
}

type ClientOption func(*clientConfig)

func WithAPIKey(key string) ClientOption {
	return func(c *clientConfig) {
		c.apiKey = key
	}
}

func WithCodex(auth *codex.Auth) ClientOption {
	return func(c *clientConfig) {
		c.codexAuth = auth
	}
}

// the client reads through this pointer on each call, so the caller
// can update the value at runtime (e.g. from config).
func WithReasoningEffort(effort *string) ClientOption {
	return func(c *clientConfig) {
		c.reasoningEffort = effort
	}
}

func WithVerbosity(v *string) ClientOption {
	return func(c *clientConfig) {
		c.verbosity = v
	}
}

func NewClient(model *string, opts ...ClientOption) *Client {
	var cfg clientConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	c := &Client{
		model:           model,
		reasoningEffort: cfg.reasoningEffort,
		verbosity:       cfg.verbosity,
		sem:             make(chan struct{}, maxConcurrentSessions),
	}

	if cfg.apiKey != "" {
		apiClient := openai.NewClient(option.WithAPIKey(cfg.apiKey))
		c.apiClient = &apiClient
	}

	if cfg.codexAuth != nil {
		auth := cfg.codexAuth
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

		codexClient := openai.NewClient(
			option.WithAPIKey("codex-oauth"),
			option.WithBaseURL("https://chatgpt.com/backend-api/codex"),
			option.WithMiddleware(mw),
			option.WithHeader("originator", "codex_cli_rs"),
		)
		c.codexClient = &codexClient
	}

	return c
}

func (c *Client) Model() string {
	return *c.model
}

type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type ToolCall struct {
	CallID    string
	Name      string
	Arguments string
}

type ToolExecutor func(ctx context.Context, call ToolCall) (string, error)

type Tool struct {
	Def        ToolDef
	Handler    ToolExecutor
	Privileged bool
}

func SplitTools(tools []Tool) ([]ToolDef, ToolExecutor) {
	defs := make([]ToolDef, len(tools))
	handlers := make(map[string]ToolExecutor, len(tools))

	for i, t := range tools {
		defs[i] = t.Def
		handlers[t.Def.Name] = t.Handler
	}

	exec := func(ctx context.Context, call ToolCall) (string, error) {
		h, ok := handlers[call.Name]
		if !ok {
			return ToolErrorf("unknown tool %q", call.Name), nil
		}
		return h(ctx, call)
	}

	return defs, exec
}

type Usage struct {
	InputTokens     int64
	OutputTokens    int64
	TotalTokens     int64
	CachedTokens    int64
	ReasoningTokens int64
}

type CompletionExtra struct {
	RawResponses       []string
	ReasoningSummaries []string
	ReasoningEffort    string
}

type ToolCallRecord struct {
	Name       string
	Args       string
	Result     string
	Error      bool
	DurationMS int64
}

type CompletionResult struct {
	Output  string
	Usage   Usage
	History []ToolCallRecord
	Extra   CompletionExtra
	Err     error
}

const (
	maxRounds     = 75
	loopThreshold = 4
)

func StaticInput(s string) func() string {
	return func() string { return s }
}

// Complete starts an LLM completion. getInput is called at the top of every
// round; its return replaces the user message (items[0]). Returns the
// activation ID and a channel that receives exactly one result.
func (c *Client) Complete(ctx context.Context, instructions string, getInput func() string, tools []ToolDef, executor ToolExecutor) (string, <-chan CompletionResult) {
	ch := make(chan CompletionResult, 1)

	client := c.apiClient
	if c.codexClient != nil {
		client = c.codexClient
	}

	if client == nil {
		ch <- CompletionResult{Err: fmt.Errorf("no client configured (need api key or codex auth)")}
		close(ch)
		return "", ch
	}

	actID := id.V7()
	ctx = augmentCtxMeta(ctx, "activation_id", actID)

	if c.observer != nil {
		c.observer.OnStart(ctx, *c.model)
	}

	go func() {
		defer close(ch)

		c.sem <- struct{}{}
		defer func() { <-c.sem }()

		result := c.completeLoop(ctx, client, instructions, getInput, tools, executor)
		ch <- result
	}()

	return actID, ch
}

func augmentCtxMeta(ctx context.Context, key, value string) context.Context {
	existing, _ := ctx.Value("meta").(map[string]string)
	m := make(map[string]string, len(existing)+1)
	for k, v := range existing {
		m[k] = v
	}
	m[key] = value
	return context.WithValue(ctx, "meta", m)
}

func (c *Client) completeLoop(ctx context.Context, client *openai.Client, instructions string, getInput func() string, tools []ToolDef, executor ToolExecutor) CompletionResult {
	total := Usage{}
	extra := CompletionExtra{}

	completeStart := time.Now()
	var retErr error
	var retOutput string
	var history []ToolCallRecord

	var lastInput string

	if c.observer != nil {
		defer func() {
			c.observer.OnFinish(ctx, *c.model, extra.ReasoningEffort, total, len(history), time.Since(completeStart).Milliseconds(), retOutput, retErr)

			toolNames := make([]string, len(tools))
			for i, t := range tools {
				toolNames[i] = t.Name
			}
			c.observer.OnDetail(ctx, instructions, lastInput, toolNames, extra.ReasoningSummaries)
		}()
	}

	var items responses.ResponseInputParam

	params := responses.ResponseNewParams{
		Model:        shared.ResponsesModel(*c.model),
		Instructions: openai.String(instructions),
		Tools:        buildToolParams(tools),
		Reasoning: shared.ReasoningParam{
			Summary: shared.ReasoningSummaryDetailed,
		},
	}

	if c.reasoningEffort != nil && *c.reasoningEffort != "" {
		params.Reasoning.Effort = shared.ReasoningEffort(*c.reasoningEffort)
	}

	if c.verbosity != nil && *c.verbosity != "" {
		params.Text = responses.ResponseTextConfigParam{
			Verbosity: responses.ResponseTextConfigVerbosity(*c.verbosity),
		}
	}

	m := *c.model
	if strings.Contains(m, "spark") || strings.Contains(m, "nano") || strings.Contains(m, "4.1-mini") {
		params.Reasoning = shared.ReasoningParam{}
		params.Text = responses.ResponseTextConfigParam{}
	}

	if c.codexClient != nil {
		params.Store = openai.Bool(false)
	}

	useStreaming := c.codexClient != nil

	var prevSig string
	var consecutiveRepeats int

	for round := 0; ; round++ {
		if getInput != nil {
			content := getInput()
			lastInput = content
			msg := responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser)
			if round == 0 {
				items = append(items, msg)
			} else {
				items[0] = msg
			}
		}

		if round >= maxRounds {
			slog.Warn("max rounds reached", "pkg", "llm", "rounds", round)
			retErr = fmt.Errorf("max rounds (%d) reached without final response", maxRounds)
			return CompletionResult{Usage: total, History: history, Extra: extra, Err: retErr}
		}
		params.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: items}

		var resp *responses.Response

		if useStreaming {
			r, err := completeStreaming(ctx, client, params)
			if err != nil {
				retErr = fmt.Errorf("complete round %d: %w", round, err)
				return CompletionResult{Usage: total, History: history, Extra: extra, Err: retErr}
			}
			resp = r
		} else {
			r, err := client.Responses.New(ctx, params)
			if err != nil {
				retErr = fmt.Errorf("complete round %d: %w", round, err)
				return CompletionResult{Usage: total, History: history, Extra: extra, Err: retErr}
			}
			resp = r
		}

		extra.RawResponses = append(extra.RawResponses, resp.RawJSON())

		if effort := string(resp.Reasoning.Effort); effort != "" {
			extra.ReasoningEffort = effort
		}

		total.InputTokens += resp.Usage.InputTokens
		total.OutputTokens += resp.Usage.OutputTokens
		total.TotalTokens += resp.Usage.TotalTokens
		total.CachedTokens += resp.Usage.InputTokensDetails.CachedTokens
		total.ReasoningTokens += resp.Usage.OutputTokensDetails.ReasoningTokens

		for _, item := range resp.Output {
			if item.Type != "reasoning" {
				continue
			}
			for _, s := range item.AsReasoning().Summary {
				if s.Text != "" {
					extra.ReasoningSummaries = append(extra.ReasoningSummaries, s.Text)
				}
			}
		}

		if resp.Status == responses.ResponseStatusIncomplete {
			reason := resp.IncompleteDetails.Reason
			slog.Warn("response incomplete", "pkg", "llm", "reason", reason, "round", round)
			retErr = fmt.Errorf("response incomplete: %s", reason)
			return CompletionResult{Usage: total, History: history, Extra: extra, Err: retErr}
		}

		var calls []ToolCall
		for _, item := range resp.Output {
			if item.Type != "function_call" {
				continue
			}

			fc := item.AsFunctionCall()
			calls = append(calls, ToolCall{
				CallID:    fc.CallID,
				Name:      fc.Name,
				Arguments: fc.Arguments,
			})
		}

		if len(calls) == 0 {
			retOutput = resp.OutputText()
			return CompletionResult{Output: retOutput, Usage: total, History: history, Extra: extra}
		}

		sig := roundSignature(calls)
		if sig == prevSig {
			consecutiveRepeats++
			if consecutiveRepeats >= loopThreshold {
				slog.Warn("loop detected", "pkg", "llm", "round", round, "repeats", consecutiveRepeats, "tool", calls[0].Name)
				retErr = fmt.Errorf("loop detected: %d identical rounds calling %s", consecutiveRepeats, calls[0].Name)
				return CompletionResult{Usage: total, History: history, Extra: extra, Err: retErr}
			}
		} else {
			consecutiveRepeats = 1
		}
		prevSig = sig

		for _, call := range calls {
			items = append(items, responses.ResponseInputItemParamOfFunctionCall(call.Arguments, call.CallID, call.Name))

			slog.Info("tool call", toolCallAttrs(ctx, call.Name, round, call.Arguments)...)

			start := time.Now()
			result, err := executor(ctx, call)
			elapsed := time.Since(start)

			rec := ToolCallRecord{
				Name:       call.Name,
				Args:       call.Arguments,
				DurationMS: elapsed.Milliseconds(),
			}

			if err != nil {
				result = ToolError(err)
				rec.Error = true
			}

			rec.Result = result
			history = append(history, rec)

			if c.observer != nil {
				c.observer.OnToolCall(ctx, rec.Name, rec.Args, rec.Result, elapsed, rec.Error)
			}

			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(call.CallID, result))
		}
	}
}

// completeStreaming uses the streaming API and collects the final completed
// response. the codex backend requires stream=true on all requests.
func completeStreaming(ctx context.Context, client *openai.Client, params responses.ResponseNewParams) (*responses.Response, error) {
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

func (c *Client) Transcribe(ctx context.Context, filePath string) (string, error) {
	if c.apiClient == nil {
		return "", fmt.Errorf("transcribe %s: requires api key", filePath)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open audio file %s: %w", filePath, err)
	}
	defer f.Close()

	resp, err := c.apiClient.Audio.Transcriptions.New(ctx, openai.AudioTranscriptionNewParams{
		File:  f,
		Model: openai.AudioModelWhisper1,
	})
	if err != nil {
		return "", fmt.Errorf("transcribe %s: %w", filePath, err)
	}

	return resp.Text, nil
}

func (c *Client) Describe(ctx context.Context, filePath, mimeType, question string) (string, error) {
	if c.apiClient == nil {
		return "", fmt.Errorf("describe %s: requires api key", filePath)
	}

	if question == "" {
		question = "Describe this content concisely."
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file %s: %w", filePath, err)
	}

	var content responses.ResponseInputMessageContentListParam

	if isImageMime(mimeType) {
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
		content = responses.ResponseInputMessageContentListParam{
			{OfInputText: &responses.ResponseInputTextParam{Text: question}},
			{OfInputImage: &responses.ResponseInputImageParam{ImageURL: param.NewOpt(dataURL)}},
		}
	} else {
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
		content = responses.ResponseInputMessageContentListParam{
			{OfInputText: &responses.ResponseInputTextParam{Text: question}},
			{OfInputFile: &responses.ResponseInputFileParam{
				FileData: param.NewOpt(dataURL),
				Filename: param.NewOpt(filepath.Base(filePath)),
			}},
		}
	}

	items := responses.ResponseInputParam{
		responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser),
	}

	resp, err := c.apiClient.Responses.New(ctx, responses.ResponseNewParams{
		Model: shared.ChatModelGPT4oMini,
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: items},
	})
	if err != nil {
		return "", fmt.Errorf("describe %s: %w", filePath, err)
	}

	return resp.OutputText(), nil
}

func isImageMime(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/")
}

func buildToolParams(tools []ToolDef) []responses.ToolUnionParam {
	params := make([]responses.ToolUnionParam, len(tools))

	for i, t := range tools {
		params[i] = responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  t.Parameters,
				Strict:      openai.Bool(true),
			},
		}
	}

	return params
}

func toolCallAttrs(ctx context.Context, name string, round int, raw string) []any {
	attrs := []any{"pkg", "llm", "tool", name, "round", round}

	if meta, ok := ctx.Value("meta").(map[string]string); ok {
		if v := meta["activation_id"]; v != "" {
			attrs = append(attrs, "activation_id", v)
		}
		if v := meta["task_id"]; v != "" {
			attrs = append(attrs, "task_id", v)
		}
	}

	var parsed map[string]any
	err := json.Unmarshal([]byte(raw), &parsed)
	if err != nil {
		return append(attrs, "args", raw)
	}

	for k, v := range parsed {
		attrs = append(attrs, k, fmt.Sprintf("%v", v))
	}

	return attrs
}

func roundSignature(calls []ToolCall) string {
	sigs := make([]string, len(calls))
	for i, c := range calls {
		sigs[i] = c.Name + "\x00" + c.Arguments
	}
	slices.Sort(sigs)
	return strings.Join(sigs, "\x01")
}
