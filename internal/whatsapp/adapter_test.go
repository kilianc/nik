package whatsapp

import "testing"

func TestNormalizeJID(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "phone bare", raw: "15551234567@s.whatsapp.net", want: "15551234567@s.whatsapp.net"},
		{name: "phone with device", raw: "15551234567:12@s.whatsapp.net", want: "15551234567@s.whatsapp.net"},
		{name: "lid bare", raw: "219971061866633@lid", want: "219971061866633@lid"},
		{name: "lid with device", raw: "219971061866633:12@lid", want: "219971061866633@lid"},
		{name: "group jid", raw: "120363123456789@g.us", want: "120363123456789@g.us"},
		{name: "invalid passthrough", raw: "not-a-jid", want: "not-a-jid"},
		{name: "empty passthrough", raw: "", want: ""},
	}

	for _, tc := range cases {
		got := normalizeJID(tc.raw)
		if got != tc.want {
			t.Fatalf("%s: normalizeJID(%q) = %q, want %q", tc.name, tc.raw, got, tc.want)
		}
	}
}

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
