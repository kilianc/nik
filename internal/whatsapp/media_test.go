package whatsapp

import "testing"

func TestNormalizeMime(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"image/jpeg; charset=utf-8", "image/jpeg"},
		{"", "application/octet-stream"},
		{"audio/ogg", "audio/ogg"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeMime(tt.input)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestExtensionFromMime(t *testing.T) {
	tests := []struct {
		mime string
		want string
	}{
		{"image/png", ".png"},
		{"application/x-unknown-type-zzz", ".bin"},
	}

	for _, tt := range tests {
		t.Run(tt.mime, func(t *testing.T) {
			got := extensionFromMime(tt.mime)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestExtractDownloadableNilMessage(t *testing.T) {
	dl, mime := extractDownloadable(nil, "image")
	if dl != nil || mime != "" {
		t.Errorf("expected nil/empty for nil message, got %v / %q", dl, mime)
	}
}
