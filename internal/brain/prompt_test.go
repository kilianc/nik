package brain

import (
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
)

func TestFormatNowIncludesTimezoneAndLocation(t *testing.T) {
	b := &Brain{
		cfg: &config.Config{
			Timezone: "UTC",
			Location: "San Francisco",
		},
	}

	out := b.formatNow(time.Date(2026, time.January, 2, 15, 4, 0, 0, time.UTC))
	if !strings.Contains(out, "## Now") {
		t.Fatalf("expected now section header, got %q", out)
	}
	if !strings.Contains(out, "Timezone: UTC") {
		t.Fatalf("expected timezone line, got %q", out)
	}
	if !strings.Contains(out, "Location: San Francisco") {
		t.Fatalf("expected location line, got %q", out)
	}
}
