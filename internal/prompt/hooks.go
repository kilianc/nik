package prompt

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type promptHook struct {
	Models  []string `yaml:"models"`
	Section string   `yaml:"section"`
	Mode    string   `yaml:"mode"`
	Content string   `yaml:"-"`
}

func loadHooks(home, model string) []promptHook {
	root, err := os.OpenRoot(home)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("open hooks root", "pkg", "prompt", "error", err)
		}
		return nil
	}
	defer root.Close()

	dirFile, err := root.Open("prompts")
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("read hooks dir", "pkg", "prompt", "error", err)
		}
		return nil
	}

	entries, err := dirFile.ReadDir(-1)
	dirFile.Close()
	if err != nil {
		slog.Warn("read hooks dir", "pkg", "prompt", "error", err)
		return nil
	}

	var hooks []promptHook

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		f, openErr := root.Open(filepath.Join("prompts", e.Name()))
		if openErr != nil {
			slog.Warn("read hook file", "pkg", "prompt", "file", e.Name(), "error", openErr)
			continue
		}

		raw, readErr := io.ReadAll(f)
		f.Close()
		if readErr != nil {
			slog.Warn("read hook file", "pkg", "prompt", "file", e.Name(), "error", readErr)
			continue
		}

		h, ok := parseHook(string(raw))
		if !ok {
			slog.Warn("parse hook frontmatter", "pkg", "prompt", "file", e.Name())
			continue
		}

		if !slices.Contains(h.Models, model) {
			continue
		}

		hooks = append(hooks, h)
	}

	return hooks
}

func parseHook(raw string) (promptHook, bool) {
	const sep = "---"

	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, sep) {
		return promptHook{}, false
	}

	rest := trimmed[len(sep):]
	idx := strings.Index(rest, sep)
	if idx < 0 {
		return promptHook{}, false
	}

	frontmatter := rest[:idx]
	body := rest[idx+len(sep):]

	var h promptHook

	err := yaml.Unmarshal([]byte(frontmatter), &h)
	if err != nil {
		return promptHook{}, false
	}

	if len(h.Models) == 0 || h.Section == "" {
		return promptHook{}, false
	}

	h.Content = strings.TrimSpace(body)
	return h, true
}

func applyHooks(content, section string, hooks []promptHook) string {
	for _, h := range hooks {
		if h.Section != section {
			continue
		}

		switch h.Mode {
		case "replace":
			content = h.Content
		default:
			content += "\n\n" + h.Content
		}
	}

	return content
}
