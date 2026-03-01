package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type Client struct {
	client *openai.Client
	model  shared.ResponsesModel
}

func NewClient(apiKey, model string) *Client {
	client := openai.NewClient(option.WithAPIKey(apiKey))
	return &Client{client: &client, model: model}
}

func (c *Client) Model() string {
	return string(c.model)
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

type Usage struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	CachedTokens int64
}

type ToolCallRecord struct {
	Name   string
	Args   string
	Result string
	Error  bool
}

func (c *Client) Think(ctx context.Context, instructions, input string, tools []ToolDef, executor ToolExecutor) (string, Usage, []ToolCallRecord, error) {
	total := Usage{}

	var history []ToolCallRecord

	items := responses.ResponseInputParam{
		responses.ResponseInputItemParamOfMessage(input, responses.EasyInputMessageRoleUser),
	}

	params := responses.ResponseNewParams{
		Model:        c.model,
		Instructions: openai.String(instructions),
		Tools:        buildToolParams(tools),
		Text: responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
			},
		},
	}

	for round := 0; ; round++ {
		params.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: items}

		resp, err := c.client.Responses.New(ctx, params)
		if err != nil {
			return "", total, history, fmt.Errorf("complete round %d: %w", round, err)
		}

		total.InputTokens += resp.Usage.InputTokens
		total.OutputTokens += resp.Usage.OutputTokens
		total.TotalTokens += resp.Usage.TotalTokens
		total.CachedTokens += resp.Usage.InputTokensDetails.CachedTokens

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
			return resp.OutputText(), total, history, nil
		}

		for _, call := range calls {
			items = append(items, responses.ResponseInputItemParamOfFunctionCall(call.Arguments, call.CallID, call.Name))

			slog.Info("tool call", "pkg", "llm", "tool", call.Name, "round", round, "args", parseToolCallArgs(call.Arguments))
			result, err := executor(ctx, call)

			rec := ToolCallRecord{
				Name: call.Name,
				Args: call.Arguments,
			}

			if err != nil {
				result = fmt.Sprintf(`{"error":%q}`, err.Error())
				rec.Error = true
			}

			rec.Result = result
			history = append(history, rec)

			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(call.CallID, result))
		}
	}

}

func (c *Client) Embed(ctx context.Context, text string) ([]float64, error) {
	resp, err := c.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: openai.EmbeddingModelTextEmbedding3Small,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String(text),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("embed text: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("embed text: no embedding returned")
	}

	return resp.Data[0].Embedding, nil
}

func (c *Client) Transcribe(ctx context.Context, filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open audio file %s: %w", filePath, err)
	}
	defer f.Close()

	resp, err := c.client.Audio.Transcriptions.New(ctx, openai.AudioTranscriptionNewParams{
		File:  f,
		Model: openai.AudioModelWhisper1,
	})
	if err != nil {
		return "", fmt.Errorf("transcribe %s: %w", filePath, err)
	}

	return resp.Text, nil
}

func (c *Client) Describe(ctx context.Context, filePath, mimeType, question string) (string, error) {
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

	resp, err := c.client.Responses.New(ctx, responses.ResponseNewParams{
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

func parseToolCallArgs(raw string) any {
	var parsed any
	err := json.Unmarshal([]byte(raw), &parsed)
	if err != nil {
		return raw
	}

	return parsed
}
