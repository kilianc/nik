package llm

type tokenPricing struct {
	input  float64
	output float64
	cached float64
}

var modelPricing = map[string]tokenPricing{
	// GPT-5.x family (cached: 90% off input)
	"gpt-5.3":            {input: 1.75e-6, output: 14.0e-6, cached: 0.175e-6},
	"gpt-5.3-codex":      {input: 1.75e-6, output: 14.0e-6, cached: 0.175e-6},
	"gpt-5.2":            {input: 1.75e-6, output: 14.0e-6, cached: 0.175e-6},
	"gpt-5.2-codex":      {input: 1.75e-6, output: 14.0e-6, cached: 0.175e-6},
	"gpt-5.1":            {input: 1.25e-6, output: 10.0e-6, cached: 0.125e-6},
	"gpt-5.1-codex":      {input: 1.25e-6, output: 10.0e-6, cached: 0.125e-6},
	"gpt-5.1-codex-max":  {input: 1.25e-6, output: 10.0e-6, cached: 0.125e-6},
	"gpt-5.1-codex-mini": {input: 0.25e-6, output: 2.00e-6, cached: 0.025e-6},
	"gpt-5":              {input: 1.25e-6, output: 10.0e-6, cached: 0.125e-6},
	"gpt-5-codex":        {input: 1.25e-6, output: 10.0e-6, cached: 0.125e-6},
	"gpt-5-mini":         {input: 0.25e-6, output: 2.00e-6, cached: 0.025e-6},
	"gpt-5-nano":         {input: 0.05e-6, output: 0.40e-6, cached: 0.005e-6},
	"gpt-5.2-pro":        {input: 21.0e-6, output: 168.0e-6},
	"gpt-5-pro":          {input: 15.0e-6, output: 120.0e-6},
	// GPT-4.1 family (cached: 75% off input)
	"gpt-4.1":      {input: 2.00e-6, output: 8.00e-6, cached: 0.50e-6},
	"gpt-4.1-mini": {input: 0.40e-6, output: 1.60e-6, cached: 0.10e-6},
	"gpt-4.1-nano": {input: 0.10e-6, output: 0.40e-6, cached: 0.025e-6},
	// GPT-4o family (cached: 50% off input)
	"gpt-4o":      {input: 2.50e-6, output: 10.0e-6, cached: 1.25e-6},
	"gpt-4o-mini": {input: 0.15e-6, output: 0.60e-6, cached: 0.075e-6},
	// Reasoning models
	"o1":      {input: 15.0e-6, output: 60.0e-6, cached: 7.50e-6},
	"o3":      {input: 2.00e-6, output: 8.00e-6, cached: 0.50e-6},
	"o3-mini": {input: 1.10e-6, output: 4.40e-6, cached: 0.55e-6},
	"o3-pro":  {input: 20.0e-6, output: 80.0e-6},
	"o4-mini": {input: 1.10e-6, output: 4.40e-6, cached: 0.275e-6},
	"o1-mini": {input: 1.10e-6, output: 4.40e-6},
}

// Rates holds per-1M-token display rates for a model.
type Rates struct {
	Input  float64
	Output float64
	Cached float64
}

// ModelRates returns the per-1M-token rates for the given model.
func ModelRates(model string) (Rates, bool) {
	p, ok := modelPricing[model]
	if !ok {
		return Rates{}, false
	}
	return Rates{
		Input:  p.input * 1e6,
		Output: p.output * 1e6,
		Cached: p.cached * 1e6,
	}, true
}

func ComputeCost(model string, inputTokens, outputTokens, cachedTokens int64) float64 {
	p, ok := modelPricing[model]
	if !ok {
		return 0
	}

	uncached := inputTokens - cachedTokens

	cachedCost := float64(cachedTokens) * p.cached
	if p.cached == 0 {
		cachedCost = float64(cachedTokens) * p.input
	}

	return float64(uncached)*p.input + cachedCost + float64(outputTokens)*p.output
}
