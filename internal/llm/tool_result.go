package llm

import (
	"encoding/json"
	"fmt"
)

func ToolError(err error) string {
	data, _ := json.Marshal(map[string]string{"error": err.Error()})
	return string(data)
}

func ToolErrorf(format string, args ...any) string {
	data, _ := json.Marshal(map[string]string{"error": fmt.Sprintf(format, args...)})
	return string(data)
}

func ToolResult(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}
