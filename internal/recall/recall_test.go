package recall

import (
	"context"
	"reflect"
	"strings"
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
	tests := []struct {
		name         string
		input        string
		wantHeader   int
		wantRows     int
		wantNumbered bool
	}{
		{
			"with data rows",
			"| date | type | entity | memory | conversation |\n|------|------|--------|--------|--------------|\n| 2026-02-13 | personal_fact | CT | phone number is +16129610041 | Terminus |\n| 2026-02-14 | preference | Kilian | prefers low-emoji responses | dm |",
			2,
			2,
			true,
		},
		{"empty", "", 0, 0, false},
		{
			"header only",
			"| date | type | entity | memory | conversation |\n|------|------|--------|--------|--------------|",
			2,
			0,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			numbered, header, rows := numberRows(tt.input)

			if len(header) != tt.wantHeader {
				t.Fatalf("expected %d header lines, got %d", tt.wantHeader, len(header))
			}

			if len(rows) != tt.wantRows {
				t.Fatalf("expected %d rows, got %d", tt.wantRows, len(rows))
			}

			if tt.wantNumbered && !strings.Contains(numbered, "1:") {
				t.Fatalf("numbered output missing IDs:\n%s", numbered)
			}
			if !tt.wantNumbered && numbered != "" {
				t.Fatalf("expected empty numbered, got %q", numbered)
			}
		})
	}
}

func TestIsSeparator(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"full dashes", "|------|------|--------|--------|--------------|", true},
		{"spaced dashes", "| --- | --- |", true},
		{"minimal dashes", "| - | - | - |", true},
		{"header row", "| date | type | entity |", false},
		{"data row", "| 2026-02-13 | personal_fact | CT |", false},
		{"not a table", "not a table row", false},
		{"empty pipes", "||", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSeparator(tt.line)
			if got != tt.want {
				t.Errorf("isSeparator(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
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
