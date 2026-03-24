package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var shorthands = map[string]string{
	"@yearly":  "0 0 1 1 *",
	"@monthly": "0 0 1 * *",
	"@weekly":  "0 0 * * 0",
	"@daily":   "0 0 * * *",
	"@hourly":  "0 * * * *",
}

type Schedule struct {
	minutes []bool // 0-59
	hours   []bool // 0-23
	doms    []bool // 1-31
	months  []bool // 1-12
	dows    []bool // 0-6 (sun=0)
}

func Parse(expr string) (*Schedule, error) {
	if s, ok := shorthands[strings.ToLower(strings.TrimSpace(expr))]; ok {
		expr = s
	}

	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d", len(parts))
	}

	minutes, err := parseField(parts[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("cron minute: %w", err)
	}

	hours, err := parseField(parts[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("cron hour: %w", err)
	}

	doms, err := parseField(parts[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("cron day-of-month: %w", err)
	}

	months, err := parseField(parts[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("cron month: %w", err)
	}

	dows, err := parseField(parts[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("cron day-of-week: %w", err)
	}

	// normalize sunday: 7 -> 0
	if len(dows) > 7 && dows[7] {
		dows[0] = true
	}

	return &Schedule{
		minutes: minutes,
		hours:   hours,
		doms:    doms,
		months:  months,
		dows:    dows[:7],
	}, nil
}

func parseField(field string, min, max int) ([]bool, error) {
	set := make([]bool, max+1)

	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)

		step := 1
		if i := strings.Index(part, "/"); i >= 0 {
			s, err := strconv.Atoi(part[i+1:])
			if err != nil || s <= 0 {
				return nil, fmt.Errorf("invalid step %q", part)
			}
			step = s
			part = part[:i]
		}

		var lo, hi int

		switch {
		case part == "*":
			lo, hi = min, max

		case strings.Contains(part, "-"):
			bounds := strings.SplitN(part, "-", 2)
			var err error
			lo, err = strconv.Atoi(bounds[0])
			if err != nil {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			hi, err = strconv.Atoi(bounds[1])
			if err != nil {
				return nil, fmt.Errorf("invalid range %q", part)
			}

		default:
			v, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid value %q", part)
			}
			lo, hi = v, v
		}

		if lo < min || hi > max || lo > hi {
			return nil, fmt.Errorf("value out of range: %d-%d (allowed %d-%d)", lo, hi, min, max)
		}

		for v := lo; v <= hi; v += step {
			set[v] = true
		}
	}

	return set, nil
}

// NextAfter returns the earliest time after t that matches the schedule.
// Searches up to 4 years to handle leap year edge cases.
func (c *Schedule) NextAfter(t time.Time) (time.Time, error) {
	t = t.Add(time.Minute).Truncate(time.Minute)
	limit := t.Add(4 * 365 * 24 * time.Hour)

	for t.Before(limit) {
		if !c.months[t.Month()] {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}

		if !c.doms[t.Day()] || !c.dows[t.Weekday()] {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}

		if !c.hours[t.Hour()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}

		if !c.minutes[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}

		return t, nil
	}

	return time.Time{}, fmt.Errorf("cron: no match within 4 years")
}

func IsValid(expr string) bool {
	_, err := Parse(expr)
	return err == nil
}
