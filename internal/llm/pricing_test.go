package llm

import (
	"math"
	"testing"
)

func TestComputeCost(t *testing.T) {
	t.Run("unknown model returns zero", func(t *testing.T) {
		cost := ComputeCost("unknown-model", 100, 100, 0)
		if cost != 0 {
			t.Fatalf("expected zero cost for unknown model, got %f", cost)
		}
	})

	t.Run("known model returns positive", func(t *testing.T) {
		cost := ComputeCost("gpt-5", 1000, 500, 0)
		if cost <= 0 {
			t.Fatalf("expected positive cost for known model, got %f", cost)
		}
	})

	t.Run("cached tokens reduce cost", func(t *testing.T) {
		full := ComputeCost("gpt-5.2-codex", 100000, 1000, 0)
		cached := ComputeCost("gpt-5.2-codex", 100000, 1000, 80000)
		if cached >= full {
			t.Fatalf("cached cost ($%.6f) should be less than full cost ($%.6f)", cached, full)
		}

		want := 20000*1.75e-6 + 80000*0.175e-6 + 1000*14.0e-6
		if math.Abs(cached-want) > 1e-9 {
			t.Fatalf("expected $%.6f, got $%.6f", want, cached)
		}
	})

	t.Run("no cache rate falls back to input", func(t *testing.T) {
		full := ComputeCost("gpt-5.2-pro", 10000, 500, 0)
		withCached := ComputeCost("gpt-5.2-pro", 10000, 500, 5000)
		if math.Abs(full-withCached) > 1e-9 {
			t.Fatalf("expected same cost, got $%.6f vs $%.6f", full, withCached)
		}
	})
}

func TestModelRates(t *testing.T) {
	t.Run("known model", func(t *testing.T) {
		rates, ok := ModelRates("gpt-5.2-codex")
		if !ok {
			t.Fatal("expected gpt-5.2-codex to be found")
		}

		if rates.Input != 1.75 {
			t.Fatalf("expected input rate 1.75, got %f", rates.Input)
		}
		if rates.Output != 14.0 {
			t.Fatalf("expected output rate 14.0, got %f", rates.Output)
		}
		if rates.Cached != 0.175 {
			t.Fatalf("expected cached rate 0.175, got %f", rates.Cached)
		}
	})

	t.Run("unknown model", func(t *testing.T) {
		_, ok := ModelRates("nonexistent")
		if ok {
			t.Fatal("expected unknown model to return false")
		}
	})
}

func TestComputeCostGPT54Codex(t *testing.T) {
	cost := ComputeCost("gpt-5.4-codex", 100000, 1000, 80000)

	want := 20000*2.50e-6 + 80000*0.25e-6 + 1000*15.0e-6
	if math.Abs(cost-want) > 1e-9 {
		t.Fatalf("expected $%.6f, got $%.6f", want, cost)
	}
}
