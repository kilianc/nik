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
		&Client{},
		"/home/nik",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
		&Client{},
		"/home/nik",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "/home/nik/media/abc123.oga") {
		t.Fatalf("expected absolute path preserved, got %q", out)
	}
}

func TestDescribeMediaBlocksPathTraversal(t *testing.T) {
	cases := []string{
		"/etc/passwd",
		"../../../etc/passwd",
		"media/../../etc/passwd",
	}
	for _, fp := range cases {
		out, err := describeMedia(
			context.Background(),
			ToolCall{Arguments: `{"file_path":"` + fp + `","question":""}`},
			&Client{},
			"/home/nik",
		)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", fp, err)
		}
		if !strings.Contains(out, "must be within") {
			t.Fatalf("expected path containment error for %q, got %q", fp, out)
		}
	}
}

func TestIsUnderDir(t *testing.T) {
	if !isUnderDir("/home/nik/media/file.ogg", "/home/nik") {
		t.Fatal("expected /home/nik/media/file.ogg under /home/nik")
	}
	if !isUnderDir("/home/nik/file.txt", "/home/nik") {
		t.Fatal("expected /home/nik/file.txt under /home/nik")
	}
	if isUnderDir("/etc/passwd", "/home/nik") {
		t.Fatal("expected /etc/passwd not under /home/nik")
	}
	if isUnderDir("/home/nik/../etc/passwd", "/home/nik") {
		t.Fatal("expected traversal path not under /home/nik")
	}
}
