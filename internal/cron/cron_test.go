package cron

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"every minute", "* * * * *", false},
		{"daily at 9am", "0 9 * * *", false},
		{"weekdays at 8:30", "30 8 * * 1-5", false},
		{"every 15 min", "*/15 * * * *", false},
		{"first of month", "0 0 1 * *", false},
		{"shorthand daily", "@daily", false},
		{"shorthand hourly", "@hourly", false},
		{"too few fields", "* * *", true},
		{"too many fields", "* * * * * *", true},
		{"invalid value", "abc * * * *", true},
		{"out of range minute", "60 * * * *", true},
		{"out of range hour", "* 25 * * *", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse(%q) err = %v, wantErr = %v", tt.expr, err, tt.wantErr)
			}
		})
	}
}

func TestNextAfter(t *testing.T) {
	loc := time.UTC

	tests := []struct {
		name string
		expr string
		from time.Time
		want time.Time
	}{
		{
			"every minute advances one",
			"* * * * *",
			time.Date(2026, 3, 12, 10, 30, 0, 0, loc),
			time.Date(2026, 3, 12, 10, 31, 0, 0, loc),
		},
		{
			"daily at 9am from 8am",
			"0 9 * * *",
			time.Date(2026, 3, 12, 8, 0, 0, 0, loc),
			time.Date(2026, 3, 12, 9, 0, 0, 0, loc),
		},
		{
			"daily at 9am from 10am rolls to next day",
			"0 9 * * *",
			time.Date(2026, 3, 12, 10, 0, 0, 0, loc),
			time.Date(2026, 3, 13, 9, 0, 0, 0, loc),
		},
		{
			"every 15 min from :07",
			"*/15 * * * *",
			time.Date(2026, 3, 12, 10, 7, 0, 0, loc),
			time.Date(2026, 3, 12, 10, 15, 0, 0, loc),
		},
		{
			"weekday from friday rolls to monday",
			"0 9 * * 1-5",
			time.Date(2026, 3, 13, 10, 0, 0, 0, loc), // friday
			time.Date(2026, 3, 16, 9, 0, 0, 0, loc),  // monday
		},
		{
			"first of month from mid-month",
			"0 0 1 * *",
			time.Date(2026, 3, 15, 0, 0, 0, 0, loc),
			time.Date(2026, 4, 1, 0, 0, 0, 0, loc),
		},
		{
			"shorthand daily",
			"@daily",
			time.Date(2026, 3, 12, 0, 1, 0, 0, loc),
			time.Date(2026, 3, 13, 0, 0, 0, 0, loc),
		},
		{
			"from mid-second truncates",
			"*/5 * * * *",
			time.Date(2026, 3, 12, 10, 3, 45, 0, loc),
			time.Date(2026, 3, 12, 10, 5, 0, 0, loc),
		},
		{
			"month boundary",
			"0 12 * * *",
			time.Date(2026, 3, 31, 13, 0, 0, 0, loc),
			time.Date(2026, 4, 1, 12, 0, 0, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched, err := Parse(tt.expr)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.expr, err)
			}

			got, err := sched.NextAfter(tt.from)
			if err != nil {
				t.Fatalf("NextAfter: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("NextAfter(%v) = %v, want %v", tt.from, got, tt.want)
			}
		})
	}
}

func TestIsValid(t *testing.T) {
	tests := []struct {
		expr string
		want bool
	}{
		{"0 9 * * *", true},
		{"@daily", true},
		{"not cron", false},
		{"5m", false},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			if got := IsValid(tt.expr); got != tt.want {
				t.Errorf("IsValid(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}
