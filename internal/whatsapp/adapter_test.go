package whatsapp

import "testing"

func TestIsUnknownEditType(t *testing.T) {
	cases := []struct {
		name     string
		editType string
		unknown  bool
	}{
		{name: "empty", editType: "", unknown: false},
		{name: "edit", editType: "1", unknown: false},
		{name: "admin edit", editType: "3", unknown: false},
		{name: "revoke", editType: "7", unknown: false},
		{name: "unsupported", editType: "999", unknown: true},
		{name: "literal unknown", editType: "unknown", unknown: true},
	}

	for _, tc := range cases {
		got := isUnknownEditType(tc.editType)
		if got != tc.unknown {
			t.Fatalf("%s: expected unknown=%t, got %t", tc.name, tc.unknown, got)
		}
	}
}
