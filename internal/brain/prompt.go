package brain

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/skills"
)

const skillInsertMarker = "## How your brain works"

func (b *Brain) loadInstructions(now time.Time) (string, error) {
	path := filepath.Join(b.cfg.PromptsPath(), "00-unified.md")

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt %s: %w", path, err)
	}

	prompt := string(data)
	skillSection := b.formatSkillSections()

	if skillSection != "" {
		if idx := strings.Index(prompt, skillInsertMarker); idx != -1 {
			prompt = prompt[:idx] + skillSection + "\n\n" + prompt[idx:]
		} else {
			prompt += "\n\n" + skillSection
		}
	}

	parts := []string{
		b.formatNow(now),
	}

	if b.soulReader != nil {
		soul, err := b.soulReader(context.Background())
		if err != nil {
			slog.Warn("load soul", "pkg", "brain", "error", err)
		} else if soul != "" {
			parts = append(parts, "## Soul\n\n"+soul)
		}
	}

	parts = append(parts, prompt)

	return strings.Join(parts, "\n\n"), nil
}

func (b *Brain) formatSkillSections() string {
	dirs := []string{b.cfg.SkillsPath(), b.cfg.WorkspaceSkillsPath()}

	summaries, err := skills.ListSkills(dirs...)
	if err != nil {
		slog.Warn("load skill index", "pkg", "brain", "error", err)
		return ""
	}

	if len(summaries) == 0 {
		return ""
	}

	var sections []string

	preloaded, err := skills.PreloadedSkills(dirs...)
	if err != nil {
		slog.Warn("load preloaded skills", "pkg", "brain", "error", err)
	}

	if len(preloaded) > 0 {
		lines := []string{"## Preloaded Skills", ""}
		for _, p := range preloaded {
			lines = append(lines, p.Content)
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}

	var nonPreloaded []string
	for _, s := range summaries {
		if s.Preload {
			continue
		}
		toolList := strings.Join(s.Tools, ", ")
		nonPreloaded = append(nonPreloaded, fmt.Sprintf("- **%s**: %s (tools: %s)", s.Name, s.Summary, toolList))
	}

	if len(nonPreloaded) > 0 {
		lines := []string{
			"## Available Skills",
			"",
			"Before using a tool, load its skill first -- it has the full instructions.",
			"",
		}
		lines = append(lines, nonPreloaded...)
		sections = append(sections, strings.Join(lines, "\n"))
	}

	return strings.Join(sections, "\n\n")
}

func (b *Brain) formatNow(now time.Time) string {
	loc := b.cfg.TZ()
	now = now.In(loc)

	abbrev, offset := now.Zone()
	hours := offset / 3600

	sign := "+"
	if hours < 0 {
		sign = ""
	}

	lines := []string{
		"## Now",
		now.Format("Monday, January 2, 2006 3:04 PM"),
		fmt.Sprintf("Timezone: %s (%s, UTC%s%d)", loc.String(), abbrev, sign, hours),
	}

	if b.cfg.Location != "" {
		lines = append(lines, fmt.Sprintf("Location: %s", b.cfg.Location))
	}

	return strings.Join(lines, "\n")
}
