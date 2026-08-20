package apisvc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/api"
)

func TestInspectorRunsReadOnlyQueries(t *testing.T) {
	_, conn := newTestChat(t)

	result, err := NewInspector(conn).Query(context.Background(), "SELECT 1 AS n")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result == nil {
		t.Fatal("no result")
	}
}

// The read-only check is the safety property of this endpoint, and it comes
// from the same code db_query uses rather than a second copy of the rules.
func TestInspectorRefusesWrites(t *testing.T) {
	_, conn := newTestChat(t)

	for _, query := range []string{
		"DELETE FROM message",
		"UPDATE contact SET name = 'x'",
		"SELECT 1; DROP TABLE message",
	} {
		_, err := NewInspector(conn).Query(context.Background(), query)
		if !errors.Is(err, api.ErrNotReadOnly) {
			t.Errorf("Query(%q) = %v, want ErrNotReadOnly", query, err)
		}
	}
}

func TestLogsTailReturnsTheLastLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nik.log")

	var content strings.Builder
	for i := range 500 {
		content.WriteString("line ")
		content.WriteString(string(rune('a' + i%26)))
		content.WriteString("\n")
	}

	err := os.WriteFile(path, []byte(content.String()), 0o644)
	if err != nil {
		t.Fatalf("write log: %v", err)
	}

	lines, err := NewLogs(path, path).Tail(context.Background(), false, 10)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(lines) != 10 {
		t.Fatalf("got %d lines, want 10", len(lines))
	}

	// The *last* ten, in order — a tail that returns the head is worse than
	// no tail, because it looks like it worked.
	want := "line " + string(rune('a'+499%26))
	if lines[len(lines)-1] != want {
		t.Fatalf("last line = %q, want %q", lines[len(lines)-1], want)
	}
}

func TestLogsTailHandlesAShortFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nik.log")

	err := os.WriteFile(path, []byte("only\ntwo\n"), 0o644)
	if err != nil {
		t.Fatalf("write log: %v", err)
	}

	lines, err := NewLogs(path, path).Tail(context.Background(), false, 100)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(lines) != 2 || lines[0] != "only" || lines[1] != "two" {
		t.Fatalf("lines = %v", lines)
	}
}

// A nik that has not written a log yet is not a broken nik.
func TestLogsTailOnAMissingFileIsEmpty(t *testing.T) {
	lines, err := NewLogs(filepath.Join(t.TempDir(), "nope.log"), "").Tail(context.Background(), false, 10)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("lines = %v, want empty", lines)
	}
}

func TestLogsErrorsOnlyReadsTheOtherFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nik.log")
	errPath := filepath.Join(dir, "nik.err.log")

	if err := os.WriteFile(logPath, []byte("routine\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	if err := os.WriteFile(errPath, []byte("something broke\n"), 0o644); err != nil {
		t.Fatalf("write err log: %v", err)
	}

	lines, err := NewLogs(logPath, errPath).Tail(context.Background(), true, 10)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(lines) != 1 || lines[0] != "something broke" {
		t.Fatalf("lines = %v, want the error log", lines)
	}
}
