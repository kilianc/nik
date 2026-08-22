package shell

import (
	"os"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/version"
)

func stampVersion(t *testing.T, number string) {
	t.Helper()
	prev := version.Number
	version.Number = number
	t.Cleanup(func() { version.Number = prev })
}

func TestDesiredImage(t *testing.T) {
	tests := []struct {
		name   string
		number string
		stock  bool
		want   string
	}{
		{"stock on a release pulls", "v0.4.4", true, publishedRepo + ":v0.4.4"},
		{"customised on a release builds", "v0.4.4", false, "nik-shell:v0.4.4"},
		{"stock on an unreleased build builds", "dev", true, "nik-shell:dev"},
		{"customised on an unreleased build builds", "dev", false, "nik-shell:dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stampVersion(t, tt.number)

			svc := newTestService(t)

			if got := svc.desiredImage(tt.stock); got != tt.want {
				t.Errorf("desiredImage(%v) = %q, want %q", tt.stock, got, tt.want)
			}
		})
	}
}

// A tag nobody published is not a tag to pull. An unstamped build is one whose
// Dockerfile is probably being worked on right now.
func TestPublishedImageIsEmptyForUnreleasedBuilds(t *testing.T) {
	stampVersion(t, "dev")

	if got := publishedImage(); got != "" {
		t.Errorf("publishedImage() = %q for an unstamped build, want empty", got)
	}
}

// nik pulling an image nobody publishes is a ten-minute build a family was
// promised they would not have to do, and nothing else would catch the drift:
// the pull fails quietly and falls back.
func TestPublishedRepoMatchesWorkflow(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/release.yaml")
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}

	if !strings.Contains(string(workflow), publishedRepo) {
		t.Fatalf("release workflow does not publish %s", publishedRepo)
	}
}
