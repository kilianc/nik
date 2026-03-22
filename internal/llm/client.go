package llm

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
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
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

const maxConcurrentActivations = 6

type Client struct {
	codexClient     *openai.Client
	apiClient       *openai.Client
	model           *string
	reasoningEffort *string
	verbosity       *string
	jsonOutput      bool
	sem             chan struct{}
}

type clientConfig struct {
	apiKey          string
	codexAuth       *codex.Auth
	reasoningEffort *string
	verbosity       *string
	jsonOutput      bool
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

func WithJSONOutput() ClientOption {
	return func(c *clientConfig) {
		c.jsonOutput = true
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
		jsonOutput:      cfg.jsonOutput,
		sem:             make(chan struct{}, maxConcurrentActivations),
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

type RoundStats struct {
	RoundCount             int
	MaxInputTokensPerRound int64
	MaxTotalTokensPerRound int64
}

type CompletionExtra struct {
	RawResponses    []string
	ReasoningEffort string
}

type ToolCallRecord struct {
	Name       string
	Args       string
	Result     string
	Error      bool
	DurationMS int64
}

const (
	maxRounds = 75

	historyBudgetFraction = 0.50
	estTokensPerPair      = 8000
	minHistoryPairs       = 10
	maxHistoryPairs       = 40
)

const jsonObjectInputHint = "Return a single json object only."

func extractInput(items responses.ResponseInputParam) string {
	var parts []string
	for _, item := range items {
		if item.OfMessage == nil {
			continue
		}
		if !item.OfMessage.Content.OfString.Valid() {
			continue
		}
		if s := item.OfMessage.Content.OfString.Value; s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n")
}

func ensureJSONInput(content string, jsonOutput bool) string {
	if !jsonOutput {
		return content
	}

	if strings.TrimSpace(content) != "" {
		return content
	}

	return jsonObjectInputHint
}

func maxPairsForModel(model string) int {
	ctx, ok := ModelContextWindow(model)
	if !ok {
		return maxHistoryPairs
	}

	pairs := int(float64(ctx) * historyBudgetFraction / estTokensPerPair)

	if pairs < minHistoryPairs {
		return minHistoryPairs
	}
	if pairs > maxHistoryPairs {
		return maxHistoryPairs
	}
	return pairs
}

func pruneItems(items responses.ResponseInputParam, maxPairs int) responses.ResponseInputParam {
	var pairCount int
	for _, item := range items[1:] {
		if item.OfFunctionCallOutput != nil {
			pairCount++
		}
	}

	if pairCount <= maxPairs {
		return items
	}

	dropPairs := pairCount - maxPairs
	var dropped int
	cutIdx := 1
	for cutIdx < len(items) && dropped < dropPairs {
		if items[cutIdx].OfFunctionCallOutput != nil {
			dropped++
		}
		cutIdx++
	}

	pruned := make(responses.ResponseInputParam, 0, len(items)-(cutIdx-1))
	pruned = append(pruned, items[0])
	pruned = append(pruned, items[cutIdx:]...)
	return pruned
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

func IsTransient(err error) bool {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 500 {
		return true
	}

	var streamErr *ssestream.StreamError
	if errors.As(err, &streamErr) {
		msg := streamErr.Message
		if strings.Contains(msg, "server_error") || strings.Contains(msg, "INTERNAL_ERROR") {
			return true
		}
	}

	return false
}

func RetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 5 * time.Second
	case 2:
		return 15 * time.Second
	default:
		return 30 * time.Second
	}
}

func (c *Client) Speech(ctx context.Context, text string, model string, voice string, instructions string, speed float64) (string, error) {
	if c.apiClient == nil {
		return "", fmt.Errorf("speech: requires api key")
	}

	speechModel := openai.SpeechModelGPT4oMiniTTS
	if strings.TrimSpace(model) != "" {
		speechModel = openai.SpeechModel(model)
	}

	params := openai.AudioSpeechNewParams{
		Input:          text,
		Model:          speechModel,
		Voice:          openai.AudioSpeechNewParamsVoice(voice),
		ResponseFormat: openai.AudioSpeechNewParamsResponseFormatOpus,
		Speed:          openai.Float(speed),
	}
	if instructions != "" {
		params.Instructions = openai.String(instructions)
	}

	resp, err := c.apiClient.Audio.Speech.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("speech tts: %w", err)
	}
	defer resp.Body.Close()

	outPath := filepath.Join(os.TempDir(), fmt.Sprintf("tts-%s.ogg", id.Short(6)))

	f, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create speech file: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return "", fmt.Errorf("write speech file: %w", err)
	}

	return outPath, nil
}

func (c *Client) Transcribe(ctx context.Context, f *os.File) (string, error) {
	if c.apiClient == nil {
		return "", fmt.Errorf("transcribe: requires api key")
	}

	resp, err := c.apiClient.Audio.Transcriptions.New(ctx, openai.AudioTranscriptionNewParams{
		File:  f,
		Model: openai.AudioModelWhisper1,
	})
	if err != nil {
		return "", fmt.Errorf("transcribe %s: %w", f.Name(), err)
	}

	return resp.Text, nil
}

func (c *Client) Describe(ctx context.Context, data []byte, filename, mimeType, question string) (string, error) {
	if c.apiClient == nil {
		return "", fmt.Errorf("describe %s: requires api key", filename)
	}

	if question == "" {
		question = "Describe this content concisely."
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
				Filename: param.NewOpt(filename),
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
		return "", fmt.Errorf("describe %s: %w", filename, err)
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

func roundSignature(calls []ToolCall) string {
	sigs := make([]string, len(calls))
	for i, c := range calls {
		sigs[i] = c.Name + "\x00" + c.Arguments
	}
	slices.Sort(sigs)
	return strings.Join(sigs, "\x01")
}
