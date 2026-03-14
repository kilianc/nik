package recall

import (
	"context"
	"reflect"
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

func TestNumberRows(t *testing.T) {
	input := `| date | type | entity | memory | conversation |
|------|------|--------|--------|--------------|
| 2026-02-13 | personal_fact | CT | phone number is +16129610041 | Terminus |
| 2026-02-14 | preference | Kilian | prefers low-emoji responses | dm |`

	numbered, rows := numberRows(input)

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0] != "| 2026-02-13 | personal_fact | CT | phone number is +16129610041 | Terminus |" {
		t.Fatalf("unexpected row 0: %q", rows[0])
	}
	if rows[1] != "| 2026-02-14 | preference | Kilian | prefers low-emoji responses | dm |" {
		t.Fatalf("unexpected row 1: %q", rows[1])
	}

	if !contains(numbered, "1:") || !contains(numbered, "2:") {
		t.Fatalf("numbered output missing IDs:\n%s", numbered)
	}
}

func TestNumberRowsEmpty(t *testing.T) {
	numbered, rows := numberRows("")
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
	if numbered != "" {
		t.Fatalf("expected empty numbered, got %q", numbered)
	}
}

func TestNumberRowsHeaderOnly(t *testing.T) {
	input := `| date | type | entity | memory | conversation |
|------|------|--------|--------|--------------|`

	_, rows := numberRows(input)
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for header-only input, got %d", len(rows))
	}
}

func TestIsSeparator(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"|------|------|--------|--------|--------------|", true},
		{"| --- | --- |", true},
		{"| - | - | - |", true},
		{"| date | type | entity |", false},
		{"| 2026-02-13 | personal_fact | CT |", false},
		{"not a table row", false},
		{"||", false},
	}

	for _, tt := range tests {
		got := isSeparator(tt.line)
		if got != tt.want {
			t.Errorf("isSeparator(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestParseSelectedIDs(t *testing.T) {
	tests := []struct {
		name   string
		output string
		maxID  int
		want   []int
	}{
		{"simple", "1,3,5", 10, []int{1, 3, 5}},
		{"with spaces", "1, 3, 5", 10, []int{1, 3, 5}},
		{"with newline", "1,3,5\n", 10, []int{1, 3, 5}},
		{"out of range", "0,1,11,3", 10, []int{1, 3}},
		{"duplicates", "1,1,3,3", 10, []int{1, 3}},
		{"nil output", "nil", 10, nil},
		{"empty output", "", 10, nil},
		{"non-numeric junk", "1,abc,3", 10, []int{1, 3}},
		{"single", "7", 10, []int{7}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSelectedIDs(tt.output, tt.maxID)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseSelectedIDs(%q, %d) = %v, want %v", tt.output, tt.maxID, got, tt.want)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
