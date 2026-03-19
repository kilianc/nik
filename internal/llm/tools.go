package llm

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

type MediaDescriptionUpdater interface {
	PersistMediaResult(ctx context.Context, localPath, text string, isTranscript bool) error
}

var describeMediaDef = ToolDef{
	Name:        "describe_media",
	Description: "Describe or transcribe a media file on disk. Persists the result automatically. For audio, transcribes. For images, documents, and stickers, describes visually. Does not support video.",
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

func BuildTools(client *Client, home string, updater MediaDescriptionUpdater) []Tool {
	return []Tool{
		{
			Def: describeMediaDef,
			Handler: func(ctx context.Context, call ToolCall) (string, error) {
				return describeMedia(ctx, call, client, home, updater)
			},
		},
	}
}

func describeMedia(ctx context.Context, call ToolCall, client *Client, home string, updater MediaDescriptionUpdater) (string, error) {
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

		persisted := persistMedia(ctx, updater, args.FilePath, text, true)
		return describeResult("transcript", text, persisted), nil
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

	persisted := persistMedia(ctx, updater, args.FilePath, text, false)
	return describeResult("description", text, persisted), nil
}

func describeResult(kind, text string, persisted bool) string {
	result := map[string]any{"ok": true, "type": kind, "persisted": persisted}
	if !persisted {
		result["text"] = text
	}
	return ToolResult(result)
}

func persistMedia(ctx context.Context, updater MediaDescriptionUpdater, localPath, text string, isTranscript bool) bool {
	if updater == nil {
		return false
	}

	err := updater.PersistMediaResult(ctx, localPath, text, isTranscript)
	if err != nil {
		slog.Warn("persist media description", "path", localPath, "error", err)
		return false
	}

	return true
}

func isAudioExt(ext string) bool {
	switch ext {
	case ".ogg", ".oga", ".mp3", ".m4a", ".wav", ".flac", ".webm", ".mpga", ".mpeg", ".mp4a":
		return true
	}
	return false
}
