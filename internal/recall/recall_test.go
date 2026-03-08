package recall

import (
	"context"
	"testing"
)

func TestRecallDisabledWhenNoClient(t *testing.T) {
	svc := &Service{}

	result := svc.Recall(context.Background(), "hello world")
	if result != "" {
		t.Fatalf("expected empty string when client is nil, got %q", result)
	}
}

func TestTokenEstimate(t *testing.T) {
	s := "hello world!" // 12 chars -> 3 tokens
	if got := tokenEstimate(s); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestPartitionSingleChunk(t *testing.T) {
	block := "line one\nline two\nline three"
	chunks := partition(block, 1_000_000)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	if chunks[0] != block {
		t.Fatalf("chunk content mismatch: %q", chunks[0])
	}
}

func TestPartitionMultipleChunks(t *testing.T) {
	var lines []string
	for i := 0; i < 10; i++ {
		lines = append(lines, "abcdefghij")
	}
	block := joinLines(lines)

	chunks := partition(block, 5)

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	reconstructed := joinChunks(chunks)
	if reconstructed != block {
		t.Fatalf("content lost in partitioning:\ngot:  %q\nwant: %q", reconstructed, block)
	}
}

func joinLines(lines []string) string {
	var b []byte
	for i, l := range lines {
		if i > 0 {
			b = append(b, '\n')
		}
		b = append(b, l...)
	}
	return string(b)
}

func joinChunks(chunks []string) string {
	var b []byte
	for i, c := range chunks {
		if i > 0 {
			b = append(b, '\n')
		}
		b = append(b, c...)
	}
	return string(b)
}
