package db

import (
	"strings"
	"testing"
)

func TestParseTableList(t *testing.T) {
	ddl := `-- people nik knows
CREATE TABLE IF NOT EXISTS contact (
  id TEXT PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS orphan (
  id TEXT PRIMARY KEY
);

-- scheduled reminders
CREATE TABLE IF NOT EXISTS alarm (
  id TEXT PRIMARY KEY
);`

	got := parseTableList(ddl)
	lines := strings.Split(got, "\n")

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), got)
	}

	if lines[0] != "- contact -- people nik knows" {
		t.Fatalf("unexpected line 0: %q", lines[0])
	}

	if lines[1] != "- orphan" {
		t.Fatalf("unexpected line 1 (should have no comment): %q", lines[1])
	}

	if lines[2] != "- alarm -- scheduled reminders" {
		t.Fatalf("unexpected line 2: %q", lines[2])
	}
}

func TestTableListUsesEmbeddedSchema(t *testing.T) {
	list := TableList()

	if !strings.Contains(list, "- contact") {
		t.Fatalf("expected contact table in list: %q", list)
	}
	if !strings.Contains(list, "- task") {
		t.Fatalf("expected task table in list: %q", list)
	}

	lines := strings.Split(list, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "- ") {
			t.Fatalf("each line should start with '- ': %q", line)
		}
		if !strings.Contains(line, " -- ") {
			t.Fatalf("each line should have a comment: %q", line)
		}
	}
}
