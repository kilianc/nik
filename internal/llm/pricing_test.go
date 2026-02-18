package llm

import "testing"

func TestComputeCostReturnsZeroForUnknownModel(t *testing.T) {
	cost := ComputeCost("unknown-model", 100, 100)
	if cost != 0 {
		t.Fatalf("expected zero cost for unknown model, got %f", cost)
	}
}

func TestComputeCostReturnsPositiveForKnownModel(t *testing.T) {
	cost := ComputeCost("gpt-5", 1000, 500)
	if cost <= 0 {
		t.Fatalf("expected positive cost for known model, got %f", cost)
	}
}
