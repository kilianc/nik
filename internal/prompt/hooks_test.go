package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseHook(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantModels  []string
		wantSection string
		wantMode    string
		wantContent string
	}{
		{
			name:        "append mode",
			raw:         "---\nmodels: [gpt-5.4, gpt-5.4-mini]\nsection: identity\nmode: append\n---\nbe shorter",
			wantModels:  []string{"gpt-5.4", "gpt-5.4-mini"},
			wantSection: "identity",
			wantMode:    "append",
			wantContent: "be shorter",
		},
		{
			name:        "replace mode",
			raw:         "---\nmodels: [o3]\nsection: brain\nmode: replace\n---\nthink differently",
			wantModels:  []string{"o3"},
			wantSection: "brain",
			wantMode:    "replace",
			wantContent: "think differently",
		},
		{
			name:        "default mode omitted",
			raw:         "---\nmodels: [gpt-5.4]\nsection: identity\n---\ncontent",
			wantModels:  []string{"gpt-5.4"},
			wantSection: "identity",
			wantMode:    "",
			wantContent: "content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, ok := parseHook(tt.raw)
			if !ok {
				t.Fatal("expected successful parse")
			}

			if len(h.Models) != len(tt.wantModels) {
				t.Fatalf("models = %v, want %v", h.Models, tt.wantModels)
			}
			for i, m := range tt.wantModels {
				if h.Models[i] != m {
					t.Fatalf("models[%d] = %q, want %q", i, h.Models[i], m)
				}
			}

			if h.Section != tt.wantSection {
				t.Fatalf("section = %q, want %q", h.Section, tt.wantSection)
			}
			if h.Mode != tt.wantMode {
				t.Fatalf("mode = %q, want %q", h.Mode, tt.wantMode)
			}
			if h.Content != tt.wantContent {
				t.Fatalf("content = %q, want %q", h.Content, tt.wantContent)
			}
		})
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

func TestApplyHooks(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		section string
		hooks   []promptHook
		want    string
	}{
		{
			name:    "append",
			base:    "base content",
			section: "identity",
			hooks:   []promptHook{{Models: []string{"gpt-5.4"}, Section: "identity", Content: "extra stuff"}},
			want:    "base content\n\nextra stuff",
		},
		{
			name:    "replace",
			base:    "old brain",
			section: "brain",
			hooks:   []promptHook{{Models: []string{"o3"}, Section: "brain", Mode: "replace", Content: "new brain"}},
			want:    "new brain",
		},
		{
			name:    "wrong section ignored",
			base:    "identity content",
			section: "identity",
			hooks:   []promptHook{{Models: []string{"gpt-5.4"}, Section: "brain", Content: "brain stuff"}},
			want:    "identity content",
		},
		{
			name:    "multiple appends",
			base:    "base",
			section: "identity",
			hooks: []promptHook{
				{Models: []string{"gpt-5.4"}, Section: "identity", Content: "first"},
				{Models: []string{"gpt-5.4"}, Section: "identity", Content: "second"},
			},
			want: "base\n\nfirst\n\nsecond",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyHooks(tt.base, tt.section, tt.hooks)
			if result != tt.want {
				t.Fatalf("got %q, want %q", result, tt.want)
			}
		})
	}
}

func TestLoadHooksModelFiltering(t *testing.T) {
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "prompts")

	err := os.Mkdir(promptsDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	writeHook := func(name, content string) {
		t.Helper()
		err := os.WriteFile(filepath.Join(promptsDir, name), []byte(content), 0o644)
		if err != nil {
			t.Fatal(err)
		}
	}

	writeHook("gpt54.md", "---\nmodels: [gpt-5.4]\nsection: identity\nmode: append\n---\ngpt54 hook")
	writeHook("o3.md", "---\nmodels: [o3]\nsection: brain\nmode: replace\n---\no3 hook")
	writeHook("shared.md", "---\nmodels: [gpt-5.4, gpt-5.4-mini]\nsection: identity\n---\nshared hook")

	tests := []struct {
		name      string
		model     string
		wantCount int
	}{
		{"gpt-5.4 gets own and shared", "gpt-5.4", 2},
		{"o3 gets only own", "o3", 1},
		{"gpt-5.4-mini gets shared only", "gpt-5.4-mini", 1},
		{"unknown model gets none", "gpt-4.1", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hooks := loadHooks(dir, tt.model)
			if len(hooks) != tt.wantCount {
				t.Fatalf("loadHooks(%q) returned %d hooks, want %d", tt.model, len(hooks), tt.wantCount)
			}
		})
	}
}

func TestLoadHooksEdgeCases(t *testing.T) {
	t.Run("missing dir returns nil", func(t *testing.T) {
		hooks := loadHooks(t.TempDir(), "gpt-5.4")
		if hooks != nil {
			t.Fatalf("expected nil for missing dir, got %v", hooks)
		}
	})

	t.Run("malformed files skipped", func(t *testing.T) {
		dir := t.TempDir()
		promptsDir := filepath.Join(dir, "prompts")

		err := os.Mkdir(promptsDir, 0o755)
		if err != nil {
			t.Fatal(err)
		}

		os.WriteFile(filepath.Join(promptsDir, "bad.md"), []byte("no frontmatter here"), 0o644)
		os.WriteFile(filepath.Join(promptsDir, "good.md"), []byte("---\nmodels: [gpt-5.4]\nsection: identity\n---\ngood content"), 0o644)

		hooks := loadHooks(dir, "gpt-5.4")
		if len(hooks) != 1 {
			t.Fatalf("expected 1 hook (bad skipped), got %d", len(hooks))
		}
		if hooks[0].Content != "good content" {
			t.Fatalf("unexpected content: %q", hooks[0].Content)
		}
	})
}
