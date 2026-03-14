package id

import (
	"encoding/hex"
	"testing"

	"github.com/google/uuid"
)

func TestV4ReturnsValidUUIDv4(t *testing.T) {
	raw := V4()

	parsed, err := uuid.Parse(raw)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}

	if parsed.Version() != 4 {
		t.Fatalf("expected version 4, got %d", parsed.Version())
	}
}

func TestV7ReturnsValidUUIDv7(t *testing.T) {
	raw := V7()

	parsed, err := uuid.Parse(raw)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}

	if parsed.Version() != 7 {
		t.Fatalf("expected version 7, got %d", parsed.Version())
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

func TestShortUniqueness(t *testing.T) {
	a := Short(4)
	b := Short(4)

	if a == b {
		t.Fatalf("two Short(4) calls returned the same value: %q", a)
	}
}

func TestShortCollision(t *testing.T) {
	seen := make(map[string]bool)
	collisions := 0

	for i := 0; i < 20; i++ {
		sid := Short(4)
		if seen[sid] {
			collisions++
		}
		seen[sid] = true
	}

	if collisions > 0 {
		t.Fatalf("got %d collisions in 20 sequential IDs", collisions)
	}
}
