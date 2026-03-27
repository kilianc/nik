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

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicopt "github.com/anthropics/anthropic-sdk-go/option"
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
	anthropicClient *anthropic.Client
	model           *string
	reasoningEffort *string
	verbosity       *string
	jsonOutput      bool
	sem             chan struct{}
}

type clientConfig struct {
	apiKey          string
	anthropicKey    string
	codexAuth       *codex.Auth
	baseURL         string
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

func WithAnthropicKey(key string) ClientOption {
	return func(c *clientConfig) {
		c.anthropicKey = key
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

func WithBaseURL(url string) ClientOption {
	return func(c *clientConfig) {
		c.baseURL = url
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
		opts := []option.RequestOption{option.WithAPIKey(cfg.apiKey)}
		if cfg.baseURL != "" {
			opts = append(opts, option.WithBaseURL(cfg.baseURL))
		}
		apiClient := openai.NewClient(opts...)
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

	if cfg.anthropicKey != "" {
		ac := anthropic.NewClient(
			anthropicopt.WithAPIKey(cfg.anthropicKey),
			anthropicopt.WithMaxRetries(0),
		)
		c.anthropicClient = &ac
	}

	return c
}

func (c *Client) Model() string {
	return *c.model
}

func (c *Client) isAnthropic() bool {
	return isAnthropicModel(*c.model)
}

func isAnthropicModel(model string) bool {
	return strings.HasPrefix(model, "claude")
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
	defaultMaxRounds = 75

	historyBudgetFraction = 0.50
	estTokensPerPair      = 4000
	minHistoryPairs       = 10
	maxHistoryPairs       = 60
)

const jsonObjectInputHint = "Return a single json object only."

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

var transientNetworkSubstrings = []string{
	"bad record MAC",
	"connection reset by peer",
	"broken pipe",
	"unexpected EOF",
	"i/o timeout",
	"tls: protocol is shutdown",
	"no such host",
}

func IsTransient(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode >= 500
	}

	var streamErr *ssestream.StreamError
	if errors.As(err, &streamErr) {
		msg := streamErr.Message
		return strings.Contains(msg, "server_error") || strings.Contains(msg, "INTERNAL_ERROR")
	}

	var anthropicErr *anthropic.Error
	if errors.As(err, &anthropicErr) {
		return anthropicErr.StatusCode >= 500 || anthropicErr.StatusCode == 429
	}

	msg := err.Error()
	for _, s := range transientNetworkSubstrings {
		if strings.Contains(msg, s) {
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

func roundSignature(calls []ToolCall) string {
	sigs := make([]string, len(calls))
	for i, c := range calls {
		sigs[i] = c.Name + "\x00" + c.Arguments
	}
	slices.Sort(sigs)
	return strings.Join(sigs, "\x01")
}
