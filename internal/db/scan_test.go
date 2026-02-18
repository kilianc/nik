package db

import (
	"strings"
	"testing"
)

func TestScanStringSliceNil(t *testing.T) {
	got, err := scanStringSlice(nil)
	if err != nil {
		t.Fatalf("scan nil slice: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil slice, got %+v", got)
	}
}

func TestScanStringSliceFromJSONString(t *testing.T) {
	got, err := scanStringSlice(`["a","b"]`)
	if err != nil {
		t.Fatalf("scan json string: %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected slice: %+v", got)
	}
}

func TestScanStringSliceEmptyJSON(t *testing.T) {
	got, err := scanStringSlice("[]")
	if err != nil {
		t.Fatalf("scan empty json: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for empty array, got %+v", got)
	}
}

func TestScanStringSliceRejectsInvalidType(t *testing.T) {
	_, err := scanStringSlice(42)
	if err == nil {
		t.Fatalf("expected type error")
	}
	if !strings.Contains(err.Error(), "expected string or []string") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarshalStringSlice(t *testing.T) {
	got := MarshalStringSlice([]string{"a", "b"})
	if got != `["a","b"]` {
		t.Fatalf("unexpected marshal: %q", got)
	}
}

func TestMarshalStringSliceNil(t *testing.T) {
	got := MarshalStringSlice(nil)
	if got != "[]" {
		t.Fatalf("expected [] for nil, got %q", got)
	}
}
