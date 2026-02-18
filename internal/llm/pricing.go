package llm

type tokenPricing struct {
	input  float64
	output float64
}

var modelPricing = map[string]tokenPricing{
	// GPT-5.x family
	"gpt-5.2":            {input: 1.75e-6, output: 14.0e-6},
	"gpt-5.2-codex":      {input: 1.75e-6, output: 14.0e-6},
	"gpt-5.1":            {input: 1.25e-6, output: 10.0e-6},
	"gpt-5.1-codex":      {input: 1.25e-6, output: 10.0e-6},
	"gpt-5.1-codex-max":  {input: 1.25e-6, output: 10.0e-6},
	"gpt-5.1-codex-mini": {input: 0.25e-6, output: 2.00e-6},
	"gpt-5":              {input: 1.25e-6, output: 10.0e-6},
	"gpt-5-codex":        {input: 1.25e-6, output: 10.0e-6},
	"gpt-5-mini":         {input: 0.25e-6, output: 2.00e-6},
	"gpt-5-nano":         {input: 0.05e-6, output: 0.40e-6},
	"gpt-5.2-pro":        {input: 21.0e-6, output: 168.0e-6},
	"gpt-5-pro":          {input: 15.0e-6, output: 120.0e-6},
	// GPT-4.1 family
	"gpt-4.1":      {input: 2.00e-6, output: 8.00e-6},
	"gpt-4.1-mini": {input: 0.40e-6, output: 1.60e-6},
	"gpt-4.1-nano": {input: 0.10e-6, output: 0.40e-6},
	// GPT-4o family
	"gpt-4o":      {input: 2.50e-6, output: 10.0e-6},
	"gpt-4o-mini": {input: 0.15e-6, output: 0.60e-6},
	// Reasoning models
	"o1":      {input: 15.0e-6, output: 60.0e-6},
	"o3":      {input: 2.00e-6, output: 8.00e-6},
	"o3-mini": {input: 1.10e-6, output: 4.40e-6},
	"o3-pro":  {input: 20.0e-6, output: 80.0e-6},
	"o4-mini": {input: 1.10e-6, output: 4.40e-6},
	"o1-mini": {input: 1.10e-6, output: 4.40e-6},
}

func ComputeCost(model string, inputTokens, outputTokens int64) float64 {
	p, ok := modelPricing[model]
	if !ok {
		return 0
	}
	return float64(inputTokens)*p.input + float64(outputTokens)*p.output
}
