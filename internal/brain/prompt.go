package brain

import (
	"fmt"
	"io"
	"log/slog"
	"os"
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
	Breath          string
	Recall          string
	WorkerTools     string
	NikTools        string
	BannedWords     []string
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
	{"identity", "nik-01-identity.md"},
	{"conversation", "nik-02-conversation.md"},
	{"skills", "nik-03-skills.md"},
	{"brain", "nik-04-brain.md"},
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

func (b *Brain) loadInstructions(now time.Time, recall string, retry bool) (string, error) {
	promptRoot, err := os.OpenRoot(b.cfg.PromptsPath())
	if err != nil {
		return "", fmt.Errorf("open prompts root: %w", err)
	}
	defer promptRoot.Close()

	baseData, err := readFromRoot(promptRoot, "nik-00-base.md")
	if err != nil {
		return "", fmt.Errorf("read prompt nik-00-base.md: %w", err)
	}

	tmpl, err := template.New("base").Funcs(templateFuncs).Parse(string(baseData))
	if err != nil {
		return "", fmt.Errorf("parse base template: %w", err)
	}

	hooks := loadHooks(b.cfg.Home, b.cfg.Models.Main.Model)

	for _, s := range sectionFiles {
		data, readErr := readFromRoot(promptRoot, s.file)
		if readErr != nil {
			return "", fmt.Errorf("read prompt %s: %w", s.file, readErr)
		}

		content := applyHooks(string(data), s.name, hooks)

		_, err = tmpl.New(s.name).Parse(content)
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

	if retry {
		nudge, nudgeErr := readFromRoot(promptRoot, "nik-05-retry.md")
		if nudgeErr != nil {
			slog.Warn("load retry nudge", "pkg", "brain", "error", nudgeErr)
		} else {
			result += "\n\n" + string(nudge)
		}
	}

	return result, nil
}

func readFromRoot(root *os.Root, name string) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(f)
}

func (b *Brain) buildPromptData(now time.Time, recall string) promptData {
	var data promptData
	data.Recall = recall

	if len(b.workerToolNames) > 0 {
		workerSet := make(map[string]bool, len(b.workerToolNames))
		backticked := make([]string, len(b.workerToolNames))
		for i, name := range b.workerToolNames {
			backticked[i] = "`" + name + "`"
			workerSet[name] = true
		}
		data.WorkerTools = strings.Join(backticked, ", ")

		var nikOnly []string
		for _, def := range b.toolDefs {
			if !workerSet[def.Name] {
				nikOnly = append(nikOnly, "`"+def.Name+"`")
			}
		}
		if len(nikOnly) > 0 {
			data.NikTools = strings.Join(nikOnly, ", ")
		}
	}

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

	data.BannedWords = b.cfg.BannedWords

	homeRoot, err := os.OpenRoot(b.cfg.Home)
	if err != nil {
		slog.Warn("open home root", "pkg", "brain", "error", err)
	} else {
		soulData, soulErr := readFromRoot(homeRoot, "soul/latest.md")
		if soulErr == nil {
			data.Soul = strings.TrimSpace(string(soulData))
		} else if !os.IsNotExist(soulErr) {
			slog.Warn("load soul", "pkg", "brain", "error", soulErr)
		}

		breathData, breathErr := readFromRoot(homeRoot, "breathing/latest.md")
		if breathErr == nil {
			data.Breath = strings.TrimSpace(string(breathData))
		} else if !os.IsNotExist(breathErr) {
			slog.Warn("load breath", "pkg", "brain", "error", breathErr)
		}

		homeRoot.Close()
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
