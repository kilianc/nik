package search

import (
	"reflect"
	"testing"
)

func TestIsReadOnlyRecognizesAllowedPrefixes(t *testing.T) {
	if !isReadOnly("select 1") {
		t.Fatalf("expected SELECT query to be read-only")
	}
	if !isReadOnly("  with x as (select 1) select * from x") {
		t.Fatalf("expected WITH query to be read-only")
	}
	if isReadOnly("delete from message") {
		t.Fatalf("expected DELETE query to be rejected")
	}
}

func TestNormalizeValueConvertsBytesAndNestedSlices(t *testing.T) {
	got := normalizeValue([]any{[]byte("x"), []any{[]byte("y")}})
	want := []any{"x", []any{"y"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
