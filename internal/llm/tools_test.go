package llm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildToolsReturnsDescribeMedia(t *testing.T) {
	tools := BuildTools(nil, "", nil)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Def.Name != "describe_media" {
		t.Fatalf("expected describe_media, got %q", tools[0].Def.Name)
	}
}

func TestIsAudioExtRecognizesSupportedExtensions(t *testing.T) {
	if !isAudioExt(".ogg") {
		t.Fatalf("expected .ogg to be supported")
	}
	if isAudioExt(".txt") {
		t.Fatalf("expected .txt to be unsupported")
	}
}

func TestDescribeMediaValidation(t *testing.T) {
	t.Run("empty file_path", func(t *testing.T) {
		out, err := describeMedia(
			context.Background(),
			ToolCall{Arguments: `{"file_path":"","question":""}`},
			nil, "", nil,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "empty file_path") {
			t.Fatalf("expected empty file_path error, got %q", out)
		}
	})

	t.Run("requires home", func(t *testing.T) {
		out, err := describeMedia(
			context.Background(),
			ToolCall{Arguments: `{"file_path":"media/test.png","question":""}`},
			nil, "", nil,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "requires a home directory") {
			t.Fatalf("expected home directory error, got %q", out)
		}
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		home := t.TempDir()
		out, err := describeMedia(
			context.Background(),
			ToolCall{Arguments: `{"file_path":"/etc/passwd","question":""}`},
			&Client{}, home, nil,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "absolute paths not allowed") {
			t.Fatalf("expected absolute path error, got %q", out)
		}
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		home := t.TempDir()
		for _, fp := range []string{"../../../etc/passwd", "media/../../etc/passwd"} {
			out, err := describeMedia(
				context.Background(),
				ToolCall{Arguments: `{"file_path":"` + fp + `","question":""}`},
				&Client{}, home, nil,
			)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", fp, err)
			}
			if !strings.Contains(out, "error") {
				t.Fatalf("expected error for %q, got %q", fp, out)
			}
		}
	})

	t.Run("symlink escape blocked", func(t *testing.T) {
		home := t.TempDir()
		outside := t.TempDir()
		os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644)
		os.Symlink(outside, filepath.Join(home, "escape"))

		out, err := describeMedia(
			context.Background(),
			ToolCall{Arguments: `{"file_path":"escape/secret.txt","question":""}`},
			&Client{}, home, nil,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "error") {
			t.Fatalf("expected error for symlink escape, got %q", out)
		}
	})
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
		&Client{}, home, nil,
	)
	if descErr != nil {
		t.Fatalf("unexpected error: %v", descErr)
	}

	if !strings.Contains(out, "requires api key") {
		t.Fatalf("expected api key error (file was opened successfully), got %q", out)
	}
}

func TestPersistMediaNilUpdater(t *testing.T) {
	ok := persistMedia(context.Background(), nil, "media/test.png", "desc", false)
	if ok {
		t.Fatalf("expected false when updater is nil")
	}
}
