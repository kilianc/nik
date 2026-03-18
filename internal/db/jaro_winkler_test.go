package db

import (
	"math"
	"testing"
)

func TestJaroWinklerSimilarity(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		wantMin float64
		wantMax float64
	}{
		{"identical", "hello", "hello", 1.0, 1.0},
		{"both empty", "", "", 1.0, 1.0},
		{"empty vs non-empty", "", "hello", 0.0, 0.0},
		{"non-empty vs empty", "hello", "", 0.0, 0.0},
		{"single identical", "a", "a", 1.0, 1.0},
		{"single different", "a", "b", 0.0, 0.0},
		{"martha/marhta", "martha", "marhta", 0.96, 1.0},
		{"dwayne/duane", "dwayne", "duane", 0.84, 1.0},
		{"dixon/dicksonx", "dixon", "dicksonx", 0.81, 1.0},
		{"no matches", "abc", "xyz", -0.01, 0.01},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := jaroWinklerSimilarity(tt.a, tt.b)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("jaroWinkler(%q, %q) = %f, want [%f, %f]", tt.a, tt.b, score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestJaroWinklerSymmetric(t *testing.T) {
	a := jaroWinklerSimilarity("test", "tset")
	b := jaroWinklerSimilarity("tset", "test")
	if math.Abs(a-b) > 1e-9 {
		t.Errorf("not symmetric: %f vs %f", a, b)
	}
}
