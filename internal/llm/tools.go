package llm

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

var describeMediaDef = ToolDef{
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

func describeMediaHandler(client *Client, home string) ToolExecutor {
	return func(ctx context.Context, call ToolCall) (string, error) {
		return describeMedia(ctx, call, client, home)
	}
}

func BuildTools(client *Client, home string) []Tool {
	return []Tool{
		{
			Def:     describeMediaDef,
			Handler: describeMediaHandler(client, home),
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
		return ToolErrorf("empty file_path"), nil
	}

	if home == "" {
		return ToolErrorf("describe_media requires a home directory"), nil
	}

	if filepath.IsAbs(args.FilePath) {
		return ToolErrorf("absolute paths not allowed, use a relative path"), nil
	}

	root, err := os.OpenRoot(home)
	if err != nil {
		return ToolError(err), nil
	}
	defer root.Close()

	ext := strings.ToLower(filepath.Ext(args.FilePath))
	mimeType := mime.TypeByExtension(ext)

	if isAudioExt(ext) {
		f, err := root.Open(args.FilePath)
		if err != nil {
			return ToolError(err), nil
		}
		defer f.Close()

		text, err := client.Transcribe(ctx, f)
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

	f, err := root.Open(args.FilePath)
	if err != nil {
		return ToolError(err), nil
	}

	data, readErr := io.ReadAll(f)
	f.Close()
	if readErr != nil {
		return ToolError(readErr), nil
	}

	text, err := client.Describe(ctx, data, filepath.Base(args.FilePath), mimeType, question)
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
