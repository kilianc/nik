package main

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/kciuffolo/nik/internal/db"
)

func TestExtractDDL(t *testing.T) {
	schema, err := os.ReadFile("../../internal/db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}

	ddl, err := extractDDL(string(schema), "message")
	if err != nil {
		t.Fatal(err)
	}

	if ddl == "" {
		t.Fatal("expected non-empty DDL")
	}

	if !contains(ddl, "media_processed") {
		t.Errorf("expected DDL to contain media_processed, got:\n%s", ddl)
	}
}

func TestExtractDDL_unknownTable(t *testing.T) {
	schema, err := os.ReadFile("../../internal/db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}

	_, err = extractDDL(string(schema), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown table")
	}
}

func TestRecreateTable(t *testing.T) {
	live, err := sql.Open("sqlite3_nik", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()

	_, err = live.Exec(`
		CREATE TABLE parent (id TEXT PRIMARY KEY);
		INSERT INTO parent VALUES ('conv1'), ('contact1');

		CREATE TABLE message (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL REFERENCES parent(id),
			contact_id TEXT NOT NULL REFERENCES parent(id),
			kind TEXT NOT NULL DEFAULT 'text' CHECK(kind IN ('text', 'audio'))
		);

		INSERT INTO message VALUES ('m1', 'conv1', 'contact1', 'text');
		INSERT INTO message VALUES ('m2', 'conv1', 'contact1', 'audio');
	`)
	if err != nil {
		t.Fatal(err)
	}

	newDDL := `CREATE TABLE message (
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL REFERENCES parent(id),
		contact_id TEXT NOT NULL REFERENCES parent(id),
		kind TEXT NOT NULL DEFAULT 'text' CHECK(kind IN ('text', 'audio', 'media_processed'))
	)`

	err = recreateTable(live, "message", newDDL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = live.Exec("INSERT INTO message VALUES ('m3', 'conv1', 'contact1', 'media_processed')")
	if err != nil {
		t.Fatalf("insert media_processed after migrate: %v", err)
	}

	var count int
	err = live.QueryRow("SELECT COUNT(*) FROM message").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 rows, got %d", count)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
