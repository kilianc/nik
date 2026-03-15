package whatsapp

import "testing"

func TestNormalizeMimeStripsParams(t *testing.T) {
	got := normalizeMime("image/jpeg; charset=utf-8")
	if got != "image/jpeg" {
		t.Errorf("expected 'image/jpeg', got %q", got)
	}
}

func TestNormalizeMimeDefaultsEmpty(t *testing.T) {
	got := normalizeMime("")
	if got != "application/octet-stream" {
		t.Errorf("expected 'application/octet-stream', got %q", got)
	}
}

func TestNormalizeMimePassthrough(t *testing.T) {
	got := normalizeMime("audio/ogg")
	if got != "audio/ogg" {
		t.Errorf("expected 'audio/ogg', got %q", got)
	}
}

func TestExtensionFromMimeKnown(t *testing.T) {
	ext := extensionFromMime("image/png")
	if ext != ".png" {
		t.Errorf("expected '.png', got %q", ext)
	}
}

func TestExtensionFromMimeUnknown(t *testing.T) {
	ext := extensionFromMime("application/x-unknown-type-zzz")
	if ext != ".bin" {
		t.Errorf("expected '.bin', got %q", ext)
	}
}

func TestExtractDownloadableNilMessage(t *testing.T) {
	dl, mime := extractDownloadable(nil, "image")
	if dl != nil || mime != "" {
		t.Errorf("expected nil/empty for nil message, got %v / %q", dl, mime)
	}
}
