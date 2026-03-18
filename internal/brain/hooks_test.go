package brain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseHook(t *testing.T) {
	raw := "---\nmodels: [gpt-5.4, gpt-5.4-mini]\nsection: identity\nmode: append\n---\nbe shorter"

	h, ok := parseHook(raw)
	if !ok {
		t.Fatal("expected successful parse")
	}

	if len(h.Models) != 2 || h.Models[0] != "gpt-5.4" || h.Models[1] != "gpt-5.4-mini" {
		t.Fatalf("unexpected models: %v", h.Models)
	}

	if h.Section != "identity" {
		t.Fatalf("unexpected section: %q", h.Section)
	}

	if h.Mode != "append" {
		t.Fatalf("unexpected mode: %q", h.Mode)
	}

	if h.Content != "be shorter" {
		t.Fatalf("unexpected content: %q", h.Content)
	}
}

func TestParseHookReplace(t *testing.T) {
	raw := "---\nmodels: [o3]\nsection: brain\nmode: replace\n---\nthink differently"

	h, ok := parseHook(raw)
	if !ok {
		t.Fatal("expected successful parse")
	}

	if h.Mode != "replace" {
		t.Fatalf("unexpected mode: %q", h.Mode)
	}

	if h.Section != "brain" {
		t.Fatalf("unexpected section: %q", h.Section)
	}
}

func TestParseHookFailures(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"missing models", "---\nsection: identity\nmode: append\n---\ncontent"},
		{"missing section", "---\nmodels: [gpt-5.4]\nmode: append\n---\ncontent"},
		{"no frontmatter", "just some markdown without frontmatter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := parseHook(tt.raw)
			if ok {
				t.Fatal("expected parse failure")
			}
		})
	}
}

func TestParseHookDefaultMode(t *testing.T) {
	raw := "---\nmodels: [gpt-5.4]\nsection: identity\n---\ncontent"

	h, ok := parseHook(raw)
	if !ok {
		t.Fatal("expected successful parse")
	}

	if h.Mode != "" {
		t.Fatalf("expected empty mode (defaults to append), got %q", h.Mode)
	}
}

func TestApplyHooksAppend(t *testing.T) {
	hooks := []promptHook{
		{Models: []string{"gpt-5.4"}, Section: "identity", Content: "extra stuff"},
	}

	result := applyHooks("base content", "identity", hooks)
	want := "base content\n\nextra stuff"
	if result != want {
		t.Fatalf("got %q, want %q", result, want)
	}
}

func TestApplyHooksReplace(t *testing.T) {
	hooks := []promptHook{
		{Models: []string{"o3"}, Section: "brain", Mode: "replace", Content: "new brain"},
	}

	result := applyHooks("old brain", "brain", hooks)
	if result != "new brain" {
		t.Fatalf("got %q, want %q", result, "new brain")
	}
}

func TestApplyHooksWrongSection(t *testing.T) {
	hooks := []promptHook{
		{Models: []string{"gpt-5.4"}, Section: "brain", Content: "brain stuff"},
	}

	result := applyHooks("identity content", "identity", hooks)
	if result != "identity content" {
		t.Fatalf("expected unchanged content, got %q", result)
	}
}

func TestApplyHooksMultiple(t *testing.T) {
	hooks := []promptHook{
		{Models: []string{"gpt-5.4"}, Section: "identity", Content: "first"},
		{Models: []string{"gpt-5.4"}, Section: "identity", Content: "second"},
	}

	result := applyHooks("base", "identity", hooks)
	want := "base\n\nfirst\n\nsecond"
	if result != want {
		t.Fatalf("got %q, want %q", result, want)
	}
}

func TestLoadHooksModelFiltering(t *testing.T) {
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "prompts")

	err := os.Mkdir(promptsDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(
		filepath.Join(promptsDir, "gpt54.md"),
		[]byte("---\nmodels: [gpt-5.4]\nsection: identity\nmode: append\n---\ngpt54 hook"),
		0o644,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(
		filepath.Join(promptsDir, "o3.md"),
		[]byte("---\nmodels: [o3]\nsection: brain\nmode: replace\n---\no3 hook"),
		0o644,
	)
	if err != nil {
		t.Fatal(err)
	}

	hooks := loadHooks(dir, "gpt-5.4")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook for gpt-5.4, got %d", len(hooks))
	}
	if hooks[0].Content != "gpt54 hook" {
		t.Fatalf("unexpected content: %q", hooks[0].Content)
	}

	hooks = loadHooks(dir, "o3")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook for o3, got %d", len(hooks))
	}
	if hooks[0].Content != "o3 hook" {
		t.Fatalf("unexpected content: %q", hooks[0].Content)
	}

	hooks = loadHooks(dir, "gpt-4.1")
	if len(hooks) != 0 {
		t.Fatalf("expected 0 hooks for gpt-4.1, got %d", len(hooks))
	}
}

func TestLoadHooksMultipleModels(t *testing.T) {
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "prompts")

	err := os.Mkdir(promptsDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(
		filepath.Join(promptsDir, "shared.md"),
		[]byte("---\nmodels: [gpt-5.4, gpt-5.4-mini]\nsection: identity\n---\nshared hook"),
		0o644,
	)
	if err != nil {
		t.Fatal(err)
	}

	hooks := loadHooks(dir, "gpt-5.4")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook for gpt-5.4, got %d", len(hooks))
	}

	hooks = loadHooks(dir, "gpt-5.4-mini")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook for gpt-5.4-mini, got %d", len(hooks))
	}
}

func TestLoadHooksNoDir(t *testing.T) {
	hooks := loadHooks(t.TempDir(), "gpt-5.4")
	if hooks != nil {
		t.Fatalf("expected nil for missing dir, got %v", hooks)
	}
}

func TestLoadHooksMalformedSkipped(t *testing.T) {
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "prompts")

	err := os.Mkdir(promptsDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(
		filepath.Join(promptsDir, "bad.md"),
		[]byte("no frontmatter here"),
		0o644,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(
		filepath.Join(promptsDir, "good.md"),
		[]byte("---\nmodels: [gpt-5.4]\nsection: identity\n---\ngood content"),
		0o644,
	)
	if err != nil {
		t.Fatal(err)
	}

	hooks := loadHooks(dir, "gpt-5.4")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook (bad skipped), got %d", len(hooks))
	}
	if hooks[0].Content != "good content" {
		t.Fatalf("unexpected content: %q", hooks[0].Content)
	}
}
