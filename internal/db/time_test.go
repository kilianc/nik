package db

import (
	"context"
	"testing"
	"time"
)

func TestISO8601MS(t *testing.T) {
	loc := time.FixedZone("PDT", -7*60*60)
	input := time.Date(2026, 3, 16, 15, 4, 5, 987654321, loc)

	got := ISO8601MS(input)

	if got != "2026-03-16T22:04:05.987Z" {
		t.Fatalf("expected canonical timestamp string, got %q", got)
	}
}

func TestIsISO8601MS(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"2026-03-16T22:04:05.123Z", true},
		{"2026-03-16 22:04:05.123+00:00", false},
		{"2026-03-16T22:04:05Z", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsISO8601MS(tt.input)
			if got != tt.want {
				t.Fatalf("IsISO8601MS(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestISO8601MSFunctionStoresCanonicalTimestamp(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	_, err = conn.Exec(`CREATE TABLE sample (at TIMESTAMP NOT NULL)`)
	if err != nil {
		t.Fatalf("create sample table: %v", err)
	}

	loc := time.FixedZone("PDT", -7*60*60)
	at := time.Date(2026, 3, 16, 15, 4, 5, 123456789, loc)

	_, err = conn.ExecContext(ctx, `INSERT INTO sample (at) VALUES (ISO8601_MS(?1))`, at)
	if err != nil {
		t.Fatalf("insert sample row: %v", err)
	}

	var raw string
	err = conn.QueryRowContext(ctx, `SELECT CAST(at AS TEXT) FROM sample`).Scan(&raw)
	if err != nil {
		t.Fatalf("select sample row: %v", err)
	}

	if raw != "2026-03-16T22:04:05.123Z" {
		t.Fatalf("expected canonical timestamp, got %q", raw)
	}
}

func TestSQLTimestampFunctions(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	var (
		now   string
		valid bool
	)
	err = conn.QueryRowContext(ctx, `SELECT NOW_ISO8601_MS(), IS_ISO8601_MS(NOW_ISO8601_MS())`).Scan(&now, &valid)
	if err != nil {
		t.Fatalf("query timestamp functions: %v", err)
	}

	if !valid {
		t.Fatal("expected NOW_ISO8601_MS output to satisfy IS_ISO8601_MS")
	}

	if !IsISO8601MS(now) {
		t.Fatalf("expected NOW_ISO8601_MS output to be canonical, got %q", now)
	}
}

func TestParseTimestampStringPreservesLocalTimezone(t *testing.T) {
	loc := time.FixedZone("PDT", -7*60*60)
	orig := time.Local
	time.Local = loc
	defer func() { time.Local = orig }()

	// round-trip: ISO8601MS serializes to UTC string, parseTimestampString reads back in time.Local
	input := time.Date(2026, 3, 29, 4, 0, 0, 0, loc)
	serialized := ISO8601MS(input)

	parsed, err := parseTimestampString(serialized)
	if err != nil {
		t.Fatalf("parseTimestampString: %v", err)
	}

	if parsed.Location() != time.Local {
		t.Errorf("location = %v, want %v", parsed.Location(), time.Local)
	}

	if parsed.Hour() != 4 {
		t.Errorf("hour = %d, want 4", parsed.Hour())
	}
}

func TestNullableISO8601MS(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	var isNull bool
	err = conn.QueryRowContext(ctx, `SELECT NULLABLE_ISO8601_MS(NULL) IS NULL`).Scan(&isNull)
	if err != nil {
		t.Fatalf("query nullable timestamp function: %v", err)
	}

	if !isNull {
		t.Fatal("expected NULLABLE_ISO8601_MS(NULL) to return NULL")
	}

	loc := time.FixedZone("PDT", -7*60*60)
	input := time.Date(2026, 3, 16, 15, 4, 5, 123456789, loc)

	var out string
	err = conn.QueryRowContext(ctx, `SELECT NULLABLE_ISO8601_MS(?1)`, input).Scan(&out)
	if err != nil {
		t.Fatalf("query nullable timestamp coercion: %v", err)
	}

	if out != "2026-03-16T22:04:05.123Z" {
		t.Fatalf("expected canonical timestamp string, got %q", out)
	}
}
