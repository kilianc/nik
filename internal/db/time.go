package db

import (
	"fmt"
	"time"
)

const timestampLayout = "2006-01-02T15:04:05.000Z"

var parseTimestampLayouts = []string{
	timestampLayout,
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05",
}

func ISO8601MS(t time.Time) string {
	if t.IsZero() {
		return t.Format(timestampLayout)
	}

	return t.UTC().Truncate(time.Millisecond).Format(timestampLayout)
}

func IsISO8601MS(value string) bool {
	parsed, err := time.Parse(timestampLayout, value)
	if err != nil {
		return false
	}

	return ISO8601MS(parsed) == value
}

func ParseTimeValue(value any) (time.Time, error) {
	switch v := value.(type) {
	case time.Time:
		return v.In(time.Local).Truncate(time.Millisecond), nil
	case string:
		return parseTimestampString(v)
	case []byte:
		return parseTimestampString(string(v))
	default:
		return time.Time{}, fmt.Errorf("unsupported time value %T", value)
	}
}

func parseTimestampString(value string) (time.Time, error) {
	for _, layout := range parseTimestampLayouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.In(time.Local).Truncate(time.Millisecond), nil
		}
	}

	return time.Time{}, fmt.Errorf("parse timestamp %q", value)
}
