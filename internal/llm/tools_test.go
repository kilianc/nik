package llm

import (
	"context"
	"strings"
	"testing"
)

func TestIsAudioExtRecognizesSupportedExtensions(t *testing.T) {
	if !isAudioExt(".ogg") {
		t.Fatalf("expected .ogg to be supported")
	}
	if isAudioExt(".txt") {
		t.Fatalf("expected .txt to be unsupported")
	}
}

func TestDescribeMediaRequiresFilePath(t *testing.T) {
	out, err := describeMedia(
		context.Background(),
		ToolCall{Arguments: `{"file_path":"","question":""}`},
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "empty file_path") {
		t.Fatalf("expected empty file_path error, got %q", out)
	}
}

func TestDescribeMediaResolvesRelativePath(t *testing.T) {
	out, err := describeMedia(
		context.Background(),
		ToolCall{Arguments: `{"file_path":"media/abc123.oga","question":""}`},
		nil,
		"/home/nik",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// nil client causes a panic-safe error; the important thing is the
	// resolved path appears in the error message, not media/media/...
	if strings.Contains(out, "media/media/") {
		t.Fatalf("path was doubled: %q", out)
	}
	if !strings.Contains(out, "/home/nik/media/abc123.oga") {
		t.Fatalf("expected resolved path in error, got %q", out)
	}
}

func TestDescribeMediaLeavesAbsolutePathAlone(t *testing.T) {
	out, err := describeMedia(
		context.Background(),
		ToolCall{Arguments: `{"file_path":"/home/nik/media/abc123.oga","question":""}`},
		nil,
		"/home/nik",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "/home/nik/media/abc123.oga") {
		t.Fatalf("expected absolute path preserved, got %q", out)
	}
}
