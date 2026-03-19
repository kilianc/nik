package llm

import (
	"context"
	"os"
	"path/filepath"
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

func TestDescribeMediaRequiresHome(t *testing.T) {
	out, err := describeMedia(
		context.Background(),
		ToolCall{Arguments: `{"file_path":"media/test.png","question":""}`},
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "requires a home directory") {
		t.Fatalf("expected home directory error, got %q", out)
	}
}

func TestDescribeMediaRejectsAbsolutePath(t *testing.T) {
	home := t.TempDir()

	out, err := describeMedia(
		context.Background(),
		ToolCall{Arguments: `{"file_path":"/etc/passwd","question":""}`},
		&Client{},
		home,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "absolute paths not allowed") {
		t.Fatalf("expected absolute path error, got %q", out)
	}
}

func TestDescribeMediaBlocksPathTraversal(t *testing.T) {
	home := t.TempDir()

	cases := []string{
		"../../../etc/passwd",
		"media/../../etc/passwd",
	}
	for _, fp := range cases {
		out, err := describeMedia(
			context.Background(),
			ToolCall{Arguments: `{"file_path":"` + fp + `","question":""}`},
			&Client{},
			home,
		)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", fp, err)
		}
		if !strings.Contains(out, "error") {
			t.Fatalf("expected error for %q, got %q", fp, out)
		}
	}
}

func TestDescribeMediaBlocksSymlinkEscape(t *testing.T) {
	home := t.TempDir()

	outside := t.TempDir()
	err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Symlink(outside, filepath.Join(home, "escape"))
	if err != nil {
		t.Fatal(err)
	}

	out, err := describeMedia(
		context.Background(),
		ToolCall{Arguments: `{"file_path":"escape/secret.txt","question":""}`},
		&Client{},
		home,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "error") {
		t.Fatalf("expected error for symlink escape, got %q", out)
	}
}

func TestDescribeMediaOpensValidFile(t *testing.T) {
	home := t.TempDir()

	err := os.MkdirAll(filepath.Join(home, "media"), 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(home, "media", "test.png"), []byte("fake-png"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	out, descErr := describeMedia(
		context.Background(),
		ToolCall{Arguments: `{"file_path":"media/test.png","question":"what is this?"}`},
		&Client{},
		home,
	)
	if descErr != nil {
		t.Fatalf("unexpected error: %v", descErr)
	}

	// Client has no apiClient, so Describe returns an error about needing an API key.
	if !strings.Contains(out, "requires api key") {
		t.Fatalf("expected api key error (file was opened successfully), got %q", out)
	}
}
