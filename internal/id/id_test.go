package id

import (
	"encoding/hex"
	"testing"

	"github.com/google/uuid"
)

func TestUUIDVersions(t *testing.T) {
	tests := []struct {
		name    string
		gen     func() string
		version uuid.Version
	}{
		{"V4", V4, 4},
		{"V7", V7, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := tt.gen()

			parsed, err := uuid.Parse(raw)
			if err != nil {
				t.Fatalf("parse uuid: %v", err)
			}

			if parsed.Version() != tt.version {
				t.Fatalf("expected version %d, got %d", tt.version, parsed.Version())
			}
		})
	}
}

func TestShortLength(t *testing.T) {
	for _, n := range []int{4, 8, 16} {
		s := Short(n)
		if len(s) != 2*n {
			t.Fatalf("Short(%d) length = %d, want %d", n, len(s), 2*n)
		}

		_, err := hex.DecodeString(s)
		if err != nil {
			t.Fatalf("Short(%d) not valid hex: %v", n, err)
		}
	}
}
