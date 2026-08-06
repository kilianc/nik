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

	t.Run("gpt-5.6-terra exact cost", func(t *testing.T) {
		cost := ComputeCost("gpt-5.6-terra", 100000, 1000, 80000)
		want := 20000*2.00e-6 + 80000*0.20e-6 + 1000*12.0e-6
		if math.Abs(cost-want) > 1e-9 {
			t.Fatalf("expected $%.6f, got $%.6f", want, cost)
		}
	})

	t.Run("gpt-5.6-sol exact cost", func(t *testing.T) {
		cost := ComputeCost("gpt-5.6-sol", 100000, 1000, 80000)
		want := 20000*5.00e-6 + 80000*0.50e-6 + 1000*30.0e-6
		if math.Abs(cost-want) > 1e-9 {
			t.Fatalf("expected $%.6f, got $%.6f", want, cost)
		}
	})

	t.Run("gpt-5.6 alias matches gpt-5.6-sol", func(t *testing.T) {
		alias := ComputeCost("gpt-5.6", 100000, 1000, 80000)
		sol := ComputeCost("gpt-5.6-sol", 100000, 1000, 80000)
		if math.Abs(alias-sol) > 1e-9 {
			t.Fatalf("expected alias cost $%.6f to match sol $%.6f", alias, sol)
		}
	})

	t.Run("gpt-5.6 family context window", func(t *testing.T) {
		for _, m := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
			got, ok := ModelContextWindow(m)
			if !ok || got != 1_050_000 {
				t.Fatalf("%s: expected 1.05M context window, got %d (ok=%v)", m, got, ok)
			}
		}
	})

	t.Run("gpt-5.4-codex exact cost", func(t *testing.T) {
		cost := ComputeCost("gpt-5.4-codex", 100000, 1000, 80000)
		want := 20000*2.50e-6 + 80000*0.25e-6 + 1000*15.0e-6
		if math.Abs(cost-want) > 1e-9 {
			t.Fatalf("expected $%.6f, got $%.6f", want, cost)
		}
	})

	t.Run("claude-opus-4-7 exact cost", func(t *testing.T) {
		cost := ComputeCost("claude-opus-4-7", 100000, 1000, 80000)
		want := 20000*5.0e-6 + 80000*0.50e-6 + 1000*25.0e-6
		if math.Abs(cost-want) > 1e-9 {
			t.Fatalf("expected $%.6f, got $%.6f", want, cost)
		}
	})

	t.Run("claude-opus-5 exact cost", func(t *testing.T) {
		cost := ComputeCost("claude-opus-5", 100000, 1000, 80000)
		want := 20000*5.0e-6 + 80000*0.50e-6 + 1000*25.0e-6
		if math.Abs(cost-want) > 1e-9 {
			t.Fatalf("expected $%.6f, got $%.6f", want, cost)
		}
	})

	t.Run("claude-opus-5 context window", func(t *testing.T) {
		got, ok := ModelContextWindow("claude-opus-5")
		if !ok || got != 1_000_000 {
			t.Fatalf("expected 1M context window, got %d (ok=%v)", got, ok)
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

	t.Run("anthropic model", func(t *testing.T) {
		rates, ok := ModelRates("claude-opus-4-6")
		if !ok {
			t.Fatal("expected claude-opus-4-6 to be found")
		}

		if rates.Input != 5.0 {
			t.Fatalf("expected input rate 5.0, got %f", rates.Input)
		}
		if rates.Output != 25.0 {
			t.Fatalf("expected output rate 25.0, got %f", rates.Output)
		}
		if rates.Cached != 0.5 {
			t.Fatalf("expected cached rate 0.5, got %f", rates.Cached)
		}
	})
}

func TestModelContextWindow(t *testing.T) {
	ctx, ok := ModelContextWindow("claude-haiku-4-5")
	if !ok {
		t.Fatal("expected claude-haiku-4-5 to have a context window")
	}
	if ctx != 200_000 {
		t.Fatalf("expected 200000 context window, got %d", ctx)
	}
}
