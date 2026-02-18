package whatsapp

import "testing"

func TestExtractDownloadableReturnsNilForNilMessage(t *testing.T) {
	downloadable, mimeType := extractDownloadable(nil, "image")
	if downloadable != nil || mimeType != "" {
		t.Fatalf("expected nil downloadable and empty mime type, got %v and %q", downloadable, mimeType)
	}
}

func TestExtensionFromMime(t *testing.T) {
	ext := extensionFromMime("image/jpeg")
	if ext == "" || ext[0] != '.' {
		t.Fatalf("expected a valid extension for image/jpeg, got %q", ext)
	}

	if ext := extensionFromMime("totally/bogus"); ext != ".bin" {
		t.Fatalf("expected .bin fallback for unknown mime, got %q", ext)
	}
}

func TestNormalizeMime(t *testing.T) {
	if m := normalizeMime(""); m != "application/octet-stream" {
		t.Fatalf("expected application/octet-stream for empty, got %q", m)
	}

	if m := normalizeMime("audio/ogg; codecs=opus"); m != "audio/ogg" {
		t.Fatalf("expected audio/ogg, got %q", m)
	}

	if m := normalizeMime("image/jpeg"); m != "image/jpeg" {
		t.Fatalf("expected image/jpeg, got %q", m)
	}
}
