package db

import (
	"math"
	"testing"
)

func TestJaroWinklerIdentical(t *testing.T) {
	score := jaroWinklerSimilarity("hello", "hello")
	if score != 1.0 {
		t.Errorf("identical strings: expected 1.0, got %f", score)
	}
}

func TestJaroWinklerEmpty(t *testing.T) {
	if s := jaroWinklerSimilarity("", "hello"); s != 0.0 {
		t.Errorf("empty vs non-empty: expected 0.0, got %f", s)
	}
	if s := jaroWinklerSimilarity("hello", ""); s != 0.0 {
		t.Errorf("non-empty vs empty: expected 0.0, got %f", s)
	}
	if s := jaroWinklerSimilarity("", ""); s != 1.0 {
		t.Errorf("both empty: expected 1.0, got %f", s)
	}
}

func TestJaroWinklerKnownPairs(t *testing.T) {
	tests := []struct {
		a, b    string
		wantMin float64
	}{
		{"martha", "marhta", 0.96},
		{"dwayne", "duane", 0.84},
		{"dixon", "dicksonx", 0.81},
		{"abc", "xyz", -0.01}, // no matches
	}

	for _, tc := range tests {
		score := jaroWinklerSimilarity(tc.a, tc.b)
		if score < tc.wantMin {
			t.Errorf("jaroWinkler(%q, %q) = %f, want >= %f", tc.a, tc.b, score, tc.wantMin)
		}
	}
}

func TestJaroWinklerSymmetric(t *testing.T) {
	a := jaroWinklerSimilarity("test", "tset")
	b := jaroWinklerSimilarity("tset", "test")
	if math.Abs(a-b) > 1e-9 {
		t.Errorf("not symmetric: %f vs %f", a, b)
	}
}

func TestJaroWinklerSingleChar(t *testing.T) {
	if s := jaroWinklerSimilarity("a", "a"); s != 1.0 {
		t.Errorf("single identical: expected 1.0, got %f", s)
	}
	if s := jaroWinklerSimilarity("a", "b"); s != 0.0 {
		t.Errorf("single different: expected 0.0, got %f", s)
	}
}
