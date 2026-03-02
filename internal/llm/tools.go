package llm

import (
	"context"
	"encoding/json"
	"mime"
	"path/filepath"
	"strings"
)

var DescribeMediaDef = ToolDef{
	Name:        "describe_media",
	Description: "Describe or transcribe a media file on disk. For audio, returns a transcript. For images, documents, and stickers, returns a visual/content description. Does not support video files. Returns the description text.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Path to the media file. Use the media= path from conversation context as-is.",
			},
			"question": map[string]any{
				"type":        "string",
				"description": "Optional specific question about the media. If empty, a generic description is generated.",
			},
		},
		"required":             []string{"file_path", "question"},
		"additionalProperties": false,
	},
}

func DescribeMediaHandler(client *Client, home string) ToolExecutor {
	return func(ctx context.Context, call ToolCall) (string, error) {
		return describeMedia(ctx, call, client, home)
	}
}

func BuildTools(client *Client, home string) []Tool {
	return []Tool{
		{
			Def:     DescribeMediaDef,
			Handler: DescribeMediaHandler(client, home),
		},
	}
}

func describeMedia(ctx context.Context, call ToolCall, client *Client, home string) (string, error) {
	var args struct {
		FilePath string `json:"file_path"`
		Question string `json:"question"`
	}

	err := json.Unmarshal([]byte(call.Arguments), &args)
	if err != nil {
		return ToolError(err), nil
	}
	if args.FilePath == "" {
		return `{"error":"empty file_path"}`, nil
	}

	if !filepath.IsAbs(args.FilePath) && home != "" {
		args.FilePath = filepath.Join(home, args.FilePath)
	}

	ext := strings.ToLower(filepath.Ext(args.FilePath))
	mimeType := mime.TypeByExtension(ext)

	if isAudioExt(ext) {
		text, err := client.Transcribe(ctx, args.FilePath)
		if err != nil {
			return ToolError(err), nil
		}
		return ToolResult(map[string]any{"type": "transcript", "text": text}), nil
	}

	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	if strings.HasPrefix(mimeType, "video/") {
		return `{"type":"unsupported","text":"video files cannot be described"}`, nil
	}

	question := args.Question
	if question == "" {
		question = "Describe this content concisely."
	}

	text, err := client.Describe(ctx, args.FilePath, mimeType, question)
	if err != nil {
		return ToolError(err), nil
	}

	return ToolResult(map[string]any{"type": "description", "text": text}), nil
}

func isAudioExt(ext string) bool {
	switch ext {
	case ".ogg", ".oga", ".mp3", ".m4a", ".wav", ".flac", ".webm", ".mpga", ".mpeg", ".mp4a":
		return true
	}
	return false
}
