package brain

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/kciuffolo/nik/internal/skills"
)

var htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->\n?`)

type promptData struct {
	Now             nowData
	Soul            string
	Crew            string
	Recall          string
	PreloadedSkills []skills.PreloadedSkill
	AvailableSkills []skillSummaryData
}

type nowData struct {
	Date     string
	Timezone string
	Location string
}

type skillSummaryData struct {
	Name    string
	Summary string
	Tools   string
}

var sectionFiles = []struct {
	name string
	file string
}{
	{"identity", "01-identity.md"},
	{"conversation", "02-conversation.md"},
	{"skills", "03-skills.md"},
	{"brain", "04-brain.md"},
}

var templateFuncs = template.FuncMap{
	"shiftHeadings": shiftHeadings,
}

// shiftHeadings increases all markdown heading levels by n.
// "## foo" with n=2 becomes "#### foo".
func shiftHeadings(n int, content string) string {
	prefix := strings.Repeat("#", n)

	var b strings.Builder
	for i, line := range strings.Split(content, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		if len(line) > 0 && line[0] == '#' {
			b.WriteString(prefix)
		}
		b.WriteString(line)
	}

	return b.String()
}

func (b *Brain) loadInstructions(now time.Time, recall string) (string, error) {
	dir := b.cfg.PromptsPath()

	baseData, err := os.ReadFile(filepath.Join(dir, "00-base.md"))
	if err != nil {
		return "", fmt.Errorf("read prompt 00-base.md: %w", err)
	}

	tmpl, err := template.New("base").Funcs(templateFuncs).Parse(string(baseData))
	if err != nil {
		return "", fmt.Errorf("parse base template: %w", err)
	}

	for _, s := range sectionFiles {
		data, readErr := os.ReadFile(filepath.Join(dir, s.file))
		if readErr != nil {
			return "", fmt.Errorf("read prompt %s: %w", s.file, readErr)
		}

		_, err = tmpl.New(s.name).Parse(string(data))
		if err != nil {
			return "", fmt.Errorf("parse %s template: %w", s.name, err)
		}
	}

	data := b.buildPromptData(now, recall)

	var buf strings.Builder

	err = tmpl.Execute(&buf, data)
	if err != nil {
		return "", fmt.Errorf("execute prompt template: %w", err)
	}

	result := htmlCommentRe.ReplaceAllString(buf.String(), "")

	return result, nil
}

func (b *Brain) buildPromptData(now time.Time, recall string) promptData {
	var data promptData
	data.Recall = recall

	loc := b.cfg.TZ()
	t := now.In(loc)
	abbrev, offset := t.Zone()
	hours := offset / 3600

	sign := "+"
	if hours < 0 {
		sign = ""
	}

	data.Now = nowData{
		Date:     t.Format("Monday, January 2, 2006 3:04 PM"),
		Timezone: fmt.Sprintf("%s (%s, UTC%s%d)", loc.String(), abbrev, sign, hours),
		Location: b.cfg.Location,
	}

	soulData, err := os.ReadFile(filepath.Join(b.cfg.Home, "soul", "latest.md"))
	if err == nil {
		data.Soul = strings.TrimSpace(string(soulData))
	} else if !os.IsNotExist(err) {
		slog.Warn("load soul", "pkg", "brain", "error", err)
	}

	if b.crewReader != nil {
		roster, err := b.crewReader(context.Background())
		if err != nil {
			slog.Warn("load crew", "pkg", "brain", "error", err)
		} else {
			data.Crew = roster
		}
	}

	dirs := []string{b.cfg.SkillsPath(), b.cfg.WorkspaceSkillsPath()}

	preloaded, err := skills.PreloadedSkills(dirs...)
	if err != nil {
		slog.Warn("load preloaded skills", "pkg", "brain", "error", err)
	}
	data.PreloadedSkills = preloaded

	summaries, err := skills.ListSkills(dirs...)
	if err != nil {
		slog.Warn("load skill index", "pkg", "brain", "error", err)
	}

	for _, s := range summaries {
		if s.Preload {
			continue
		}
		data.AvailableSkills = append(data.AvailableSkills, skillSummaryData{
			Name:    s.Name,
			Summary: s.Summary,
			Tools:   strings.Join(s.Tools, ", "),
		})
	}

	return data
}
