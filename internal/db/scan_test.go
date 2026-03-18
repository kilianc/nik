package db

import (
	"strings"
	"testing"
)

func TestScanStringSlice(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    []string
		wantErr string
	}{
		{"nil", nil, nil, ""},
		{"json array", `["a","b"]`, []string{"a", "b"}, ""},
		{"empty json", "[]", nil, ""},
		{"invalid type", 42, nil, "expected string or []string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scanStringSlice(tt.input)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.want == nil && got != nil {
				t.Fatalf("expected nil, got %+v", got)
			}
			if tt.want != nil {
				if len(got) != len(tt.want) {
					t.Fatalf("expected %d elements, got %d", len(tt.want), len(got))
				}
				for i := range tt.want {
					if got[i] != tt.want[i] {
						t.Fatalf("element %d: expected %q, got %q", i, tt.want[i], got[i])
					}
				}
			}
		})
	}
}

func TestMarshalStringSlice(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{"normal", []string{"a", "b"}, `["a","b"]`},
		{"nil", nil, "[]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarshalStringSlice(tt.input)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
