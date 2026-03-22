package log

import (
	"encoding/json"
	"fmt"
)

func ToolCallAttrs(ctx interface{ Value(any) any }, pkg, name string, round int, raw string) []any {
	attrs := []any{"pkg", pkg, "tool", name, "round", round}

	if meta, ok := ctx.Value("meta").(map[string]string); ok {
		for k, v := range meta {
			attrs = append(attrs, k, v)
		}
	}

	var parsed map[string]any
	err := json.Unmarshal([]byte(raw), &parsed)
	if err != nil {
		return append(attrs, "args", raw)
	}

	for k, v := range parsed {
		attrs = append(attrs, k, fmt.Sprintf("%v", v))
	}

	return attrs
}
