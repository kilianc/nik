package main

import (
	"sort"
	"testing"
)

func TestExtractConstraints(t *testing.T) {
	ddl := `CREATE TABLE message (
		id TEXT PRIMARY KEY,
		kind TEXT NOT NULL DEFAULT 'text' CHECK(kind IN ('text', 'audio', 'media_processed')),
		platform TEXT NOT NULL CHECK(platform IN ('whatsapp', 'system')),
		sent_at TIMESTAMP NOT NULL,
		CHECK (IS_ISO8601_MS(sent_at)),
		UNIQUE(platform, external_message_id)
	)`

	got := extractConstraints(ddl)
	sort.Strings(got)

	expected := []string{
		"CHECK (IS_ISO8601_MS(sent_at))",
		"CHECK(kind IN ('text', 'audio', 'media_processed'))",
		"CHECK(platform IN ('whatsapp', 'system'))",
		"UNIQUE(platform, external_message_id)",
	}
	sort.Strings(expected)

	if len(got) != len(expected) {
		t.Fatalf("expected %d constraints, got %d: %v", len(expected), len(got), got)
	}

	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("constraint %d:\n  expected: %s\n  got:      %s", i, expected[i], got[i])
		}
	}
}

func TestExtractConstraints_alterTableReorder(t *testing.T) {
	// ALTER TABLE adds columns at end; sqlite_master concatenates them
	// on one line. Constraints should still match.
	desired := `CREATE TABLE t (
		id TEXT PRIMARY KEY,
		score INTEGER NOT NULL CHECK(score BETWEEN 1 AND 5),
		extra TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		CHECK (IS_ISO8601_MS(created_at))
	)`

	live := `CREATE TABLE t (
		id TEXT PRIMARY KEY,
		score INTEGER NOT NULL CHECK(score BETWEEN 1 AND 5),
		created_at TIMESTAMP NOT NULL, extra TEXT NOT NULL DEFAULT '',
		CHECK (IS_ISO8601_MS(created_at))
	)`

	dc := extractConstraints(desired)
	lc := extractConstraints(live)
	sort.Strings(dc)
	sort.Strings(lc)

	if len(dc) != len(lc) {
		t.Fatalf("constraint count differs: desired=%d live=%d", len(dc), len(lc))
	}

	for i := range dc {
		if dc[i] != lc[i] {
			t.Errorf("constraint %d differs:\n  desired: %s\n  live:    %s", i, dc[i], lc[i])
		}
	}
}

func TestExtractConstraints_checkDrift(t *testing.T) {
	desired := `CREATE TABLE message (
		kind TEXT NOT NULL DEFAULT 'text' CHECK(kind IN ('text', 'audio', 'media_processed')),
		CHECK (IS_ISO8601_MS(sent_at))
	)`

	live := `CREATE TABLE message (
		kind TEXT NOT NULL DEFAULT 'text' CHECK(kind IN ('text', 'audio')),
		CHECK (IS_ISO8601_MS(sent_at))
	)`

	dc := extractConstraints(desired)
	lc := extractConstraints(live)

	desiredSet := make(map[string]bool, len(dc))
	for _, c := range dc {
		desiredSet[c] = true
	}

	liveSet := make(map[string]bool, len(lc))
	for _, c := range lc {
		liveSet[c] = true
	}

	var missing []string
	for _, c := range dc {
		if !liveSet[c] {
			missing = append(missing, c)
		}
	}

	if len(missing) == 0 {
		t.Fatal("expected to detect missing media_processed constraint")
	}

	found := false
	for _, m := range missing {
		if contains(m, "media_processed") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected missing constraint to mention media_processed, got: %v", missing)
	}
}

func TestExtractBody(t *testing.T) {
	ddl := `CREATE TABLE t (id TEXT, name TEXT)`
	got := extractBody(ddl)
	if got != "id TEXT, name TEXT" {
		t.Errorf("expected 'id TEXT, name TEXT', got %q", got)
	}
}

func TestSplitTopLevel(t *testing.T) {
	input := "id TEXT, kind TEXT CHECK(kind IN ('a', 'b')), CHECK (foo(x))"
	got := splitTopLevel(input)
	expected := []string{
		"id TEXT",
		"kind TEXT CHECK(kind IN ('a', 'b'))",
		"CHECK (foo(x))",
	}
	if len(got) != len(expected) {
		t.Fatalf("expected %d parts, got %d: %v", len(expected), len(got), got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("part %d: expected %q, got %q", i, expected[i], got[i])
		}
	}
}

func TestCollapseWhitespace(t *testing.T) {
	input := `  CHECK(kind  IN
		('a',  'b'))  `
	got := collapseWhitespace(input)
	expected := "CHECK(kind IN ('a', 'b'))"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
