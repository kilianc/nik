package db

import (
	"encoding/json"
	"fmt"
)

type scanner interface {
	Scan(dest ...any) error
}

// scanStringSlice converts a JSON array TEXT column to []string.
func scanStringSlice(val any) ([]string, error) {
	if val == nil {
		return nil, nil
	}

	switch v := val.(type) {
	case string:
		if v == "" || v == "[]" {
			return nil, nil
		}
		var result []string
		err := json.Unmarshal([]byte(v), &result)
		if err != nil {
			return nil, fmt.Errorf("unmarshal json array: %w", err)
		}
		return result, nil

	case []byte:
		if len(v) == 0 {
			return nil, nil
		}
		var result []string
		err := json.Unmarshal(v, &result)
		if err != nil {
			return nil, fmt.Errorf("unmarshal json array: %w", err)
		}
		return result, nil

	case []string:
		return v, nil

	default:
		return nil, fmt.Errorf("expected string or []string for json array column, got %T", val)
	}
}

func MarshalStringSlice(s []string) string {
	if s == nil {
		return "[]"
	}
	b, _ := json.Marshal(s)
	return string(b)
}
