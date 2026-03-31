package prompt

import (
	"embed"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/kciuffolo/nik/internal/config"
)

//go:embed *.md
var embedded embed.FS

var htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->\n?`)

var brainSections = []struct {
	name string
	file string
}{
	{"identity", "nik-01-identity.md"},
	{"conversation", "nik-02-conversation.md"},
	{"skills", "nik-03-skills.md"},
	{"brain", "nik-04-brain.md"},
}

type Renderer struct {
	cfg *config.Config
}

func NewRenderer(cfg *config.Config) *Renderer {
	return &Renderer{cfg: cfg}
}

func (r *Renderer) Brain(d BrainData) string {
	baseData := mustRead("nik-00-base.md")
	tmpl := template.Must(template.New("base").Funcs(r.funcMap()).Parse(string(baseData)))

	hooks := loadHooks(r.cfg.Home, d.Model)

	for _, s := range brainSections {
		raw := mustRead(s.file)
		content := applyHooks(string(raw), s.name, hooks)
		template.Must(tmpl.New(s.name).Parse(content))
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, d); err != nil {
		panic(fmt.Sprintf("execute brain prompt: %v", err))
	}

	return htmlCommentRe.ReplaceAllString(buf.String(), "")
}

func (r *Renderer) Task(d TaskData) string {
	raw := mustRead("task-00.md")
	tmpl := template.Must(template.New("task").Funcs(r.funcMap()).Parse(string(raw)))

	if d.Profile == "nik" {
		identityRaw := mustRead("nik-01-identity.md")
		template.Must(tmpl.New("identity").Parse(string(identityRaw)))
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, d); err != nil {
		panic(fmt.Sprintf("execute task prompt: %v", err))
	}

	return buf.String()
}

func (r *Renderer) Input(d InputData) string {
	return r.renderSimple("nik-06-input.md", d)
}

func (r *Renderer) Nudge(name string, data any) string {
	return r.renderSimple(name, data)
}

func (r *Renderer) renderSimple(name string, data any) string {
	raw := mustRead(name)
	tmpl := template.Must(template.New(name).Funcs(r.funcMap()).Parse(string(raw)))

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("execute %s: %v", name, err))
	}

	return buf.String()
}

func (r *Renderer) funcMap() template.FuncMap {
	return template.FuncMap{
		"shiftHeadings": shiftHeadings,
		"soul":          r.loadSoul,
		"now":           r.formatNow,
		"join":          strings.Join,
		"backtickList": func(items []string) string {
			b := make([]string, len(items))
			for i, s := range items {
				b[i] = "`" + s + "`"
			}
			return strings.Join(b, ", ")
		},
	}
}

func (r *Renderer) loadSoul() string {
	root, err := os.OpenRoot(r.cfg.Home)
	if err != nil {
		return ""
	}
	defer root.Close()

	data, err := readFromRoot(root, "soul/latest.md")
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

func (r *Renderer) TTS() string {
	override := r.cfg.Home + "/prompts/tts-00.md"
	if data, err := os.ReadFile(override); err == nil {
		return strings.TrimSpace(string(data))
	}

	return strings.TrimSpace(string(mustRead("tts-00.md")))
}

func (r *Renderer) formatNow() string {
	s := time.Now().Format("Monday, January 2, 2006 3:04 PM MST")
	if r.cfg.Location != "" {
		s += " — " + r.cfg.Location
	}
	return s
}

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

func mustRead(name string) []byte {
	data, err := embedded.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("read embedded prompt %s: %v", name, err))
	}
	return data
}

func readFromRoot(root *os.Root, name string) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(f)
}
