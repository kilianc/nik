package llm

import "testing"

func TestIsImageMime(t *testing.T) {
	if !isImageMime("image/png") {
		t.Fatalf("expected image/png to be recognized as image mime")
	}
	if isImageMime("audio/ogg") {
		t.Fatalf("expected audio/ogg to not be recognized as image mime")
	}
}

func TestRoundSignature(t *testing.T) {
	a := ToolCall{Name: "load_skill", Arguments: `{"action":"load","name":"search"}`}
	b := ToolCall{Name: "search_memory", Arguments: `{"query":"hello"}`}

	sig1 := roundSignature([]ToolCall{a})
	sig2 := roundSignature([]ToolCall{a})
	if sig1 != sig2 {
		t.Fatalf("identical calls should produce identical signatures")
	}

	sig3 := roundSignature([]ToolCall{a, b})
	sig4 := roundSignature([]ToolCall{b, a})
	if sig3 != sig4 {
		t.Fatalf("order should not matter: %q != %q", sig3, sig4)
	}

	different := ToolCall{Name: "load_skill", Arguments: `{"action":"load","name":"alarm"}`}
	sig5 := roundSignature([]ToolCall{different})
	if sig1 == sig5 {
		t.Fatalf("different args should produce different signatures")
	}
}

func TestBuildToolParamsIncludesDefinitions(t *testing.T) {
	params := buildToolParams([]ToolDef{
		{
			Name:        "test_tool",
			Description: "test",
			Parameters: map[string]any{
				"type": "object",
			},
		},
	})

	if len(params) != 1 {
		t.Fatalf("expected 1 tool param, got %d", len(params))
	}
	if params[0].OfFunction == nil || params[0].OfFunction.Name != "test_tool" {
		t.Fatalf("expected function tool named test_tool, got %+v", params[0])
	}
}
