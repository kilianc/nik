package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "test_skill")
	os.MkdirAll(skillDir, 0o755)

	content := `---
name: test_skill
summary: >
  Load this skill to test frontmatter parsing.
tools: [tool_a, tool_b]
---

# Test Skill

Some content here.
`
	path := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(path, []byte(content), 0o644)

	s, err := parseFrontmatter(path)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}

	if s.Name != "test_skill" {
		t.Errorf("name = %q, want %q", s.Name, "test_skill")
	}

	if s.Summary != "Load this skill to test frontmatter parsing." {
		t.Errorf("summary = %q, want %q", s.Summary, "Load this skill to test frontmatter parsing.")
	}

	if len(s.Tools) != 2 || s.Tools[0] != "tool_a" || s.Tools[1] != "tool_b" {
		t.Errorf("tools = %v, want [tool_a tool_b]", s.Tools)
	}
}

func TestParseFrontmatterBlockTools(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "block")
	os.MkdirAll(skillDir, 0o755)

	content := `---
name: block
summary: block test
tools:
  - alpha
  - beta
  - gamma
---

# Block
`
	path := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(path, []byte(content), 0o644)

	s, err := parseFrontmatter(path)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}

	if len(s.Tools) != 3 {
		t.Fatalf("tools len = %d, want 3", len(s.Tools))
	}

	want := []string{"alpha", "beta", "gamma"}
	for i, w := range want {
		if s.Tools[i] != w {
			t.Errorf("tools[%d] = %q, want %q", i, s.Tools[i], w)
		}
	}
}

func TestListSkills(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"skill_a", "skill_b"} {
		skillDir := filepath.Join(dir, name)
		os.MkdirAll(skillDir, 0o755)
		content := "---\nname: " + name + "\nsummary: desc for " + name + "\ntools: [t1]\n---\n"
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)
	}

	summaries, err := ListSkills(dir)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("got %d summaries, want 2", len(summaries))
	}
}

func TestParseFrontmatterPreload(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "preloaded")
	os.MkdirAll(skillDir, 0o755)

	content := `---
name: preloaded
preload: true
summary: a preloaded skill
tools: [tool_x]
---

# Preloaded

Body content.
`
	path := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(path, []byte(content), 0o644)

	s, err := parseFrontmatter(path)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}

	if !s.Preload {
		t.Error("preload = false, want true")
	}

	if s.Name != "preloaded" {
		t.Errorf("name = %q, want %q", s.Name, "preloaded")
	}
}

func TestParseFrontmatterPreloadDefaultFalse(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "normal")
	os.MkdirAll(skillDir, 0o755)

	content := `---
name: normal
summary: no preload field
tools: [t1]
---

# Normal
`
	path := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(path, []byte(content), 0o644)

	s, err := parseFrontmatter(path)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}

	if s.Preload {
		t.Error("preload = true, want false (default)")
	}
}

func TestPreloadedSkills(t *testing.T) {
	dir := t.TempDir()

	// preloaded skill
	preDir := filepath.Join(dir, "pre")
	os.MkdirAll(preDir, 0o755)
	os.WriteFile(filepath.Join(preDir, "SKILL.md"), []byte(`---
name: pre
preload: true
summary: preloaded
tools: [t1]
---

# Pre

Preloaded body.
`), 0o644)

	// non-preloaded skill
	normalDir := filepath.Join(dir, "normal")
	os.MkdirAll(normalDir, 0o755)
	os.WriteFile(filepath.Join(normalDir, "SKILL.md"), []byte(`---
name: normal
summary: not preloaded
tools: [t2]
---

# Normal

Normal body.
`), 0o644)

	result, err := PreloadedSkills(dir)
	if err != nil {
		t.Fatalf("PreloadedSkills: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("got %d preloaded skills, want 1", len(result))
	}

	if result[0].Name != "pre" {
		t.Errorf("name = %q, want %q", result[0].Name, "pre")
	}

	if !strings.Contains(result[0].Content, "Preloaded body.") {
		t.Errorf("content missing body, got: %q", result[0].Content)
	}

	if strings.Contains(result[0].Content, "---") {
		t.Errorf("content should not contain frontmatter delimiters, got: %q", result[0].Content)
	}
}

func TestStripFrontmatter(t *testing.T) {
	input := "---\nname: test\n---\n\n# Body\n\nContent.\n"
	got := stripFrontmatter(input)
	want := "# Body\n\nContent.\n"

	if got != want {
		t.Errorf("stripFrontmatter = %q, want %q", got, want)
	}
}

func TestParseFlowSequence(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"[a, b, c]", []string{"a", "b", "c"}},
		{"[single]", []string{"single"}},
		{"[]", nil},
	}

	for _, tt := range tests {
		got := parseFlowSequence(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseFlowSequence(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseFlowSequence(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestListSkillsMultipleDirs(t *testing.T) {
	builtinDir := t.TempDir()
	workspaceDir := t.TempDir()

	writeSkill(t, builtinDir, "alarm", "alarm", "manage alarms", "[create_alarm]")
	writeSkill(t, builtinDir, "search", "search", "search things", "[db_query]")
	writeSkill(t, workspaceDir, "custom", "custom", "nik-authored skill", "[shell]")

	summaries, err := ListSkills(builtinDir, workspaceDir)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	if len(summaries) != 3 {
		t.Fatalf("got %d summaries, want 3", len(summaries))
	}

	names := map[string]bool{}
	for _, s := range summaries {
		names[s.Name] = true
	}

	for _, want := range []string{"alarm", "search", "custom"} {
		if !names[want] {
			t.Errorf("missing skill %q", want)
		}
	}
}

func TestListSkillsWorkspaceOverridesBuiltin(t *testing.T) {
	builtinDir := t.TempDir()
	workspaceDir := t.TempDir()

	writeSkill(t, builtinDir, "alarm", "alarm", "builtin alarms", "[create_alarm]")
	writeSkill(t, workspaceDir, "alarm", "alarm", "custom alarms", "[create_alarm, delete_alarm]")

	summaries, err := ListSkills(builtinDir, workspaceDir)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	if len(summaries) != 1 {
		t.Fatalf("got %d summaries, want 1 (deduped)", len(summaries))
	}

	if summaries[0].Summary != "custom alarms" {
		t.Errorf("summary = %q, want workspace override %q", summaries[0].Summary, "custom alarms")
	}
}

func TestListSkillsMissingDirTolerated(t *testing.T) {
	builtinDir := t.TempDir()
	writeSkill(t, builtinDir, "alarm", "alarm", "manage alarms", "[create_alarm]")

	missingDir := filepath.Join(t.TempDir(), "does_not_exist")

	summaries, err := ListSkills(builtinDir, missingDir)
	if err != nil {
		t.Fatalf("ListSkills with missing dir: %v", err)
	}

	if len(summaries) != 1 {
		t.Fatalf("got %d summaries, want 1", len(summaries))
	}
}

func TestPreloadedSkillsMultipleDirs(t *testing.T) {
	builtinDir := t.TempDir()
	workspaceDir := t.TempDir()

	writePreloadSkill(t, builtinDir, "messaging", "Builtin messaging body.")
	writeSkill(t, workspaceDir, "custom", "custom", "not preloaded", "[shell]")
	writePreloadSkill(t, workspaceDir, "ws_pre", "Workspace preloaded body.")

	result, err := PreloadedSkills(builtinDir, workspaceDir)
	if err != nil {
		t.Fatalf("PreloadedSkills: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("got %d preloaded skills, want 2", len(result))
	}

	names := map[string]string{}
	for _, p := range result {
		names[p.Name] = p.Content
	}

	if !strings.Contains(names["messaging"], "Builtin messaging body.") {
		t.Error("missing builtin preloaded skill")
	}

	if !strings.Contains(names["ws_pre"], "Workspace preloaded body.") {
		t.Error("missing workspace preloaded skill")
	}
}

func TestPreloadedSkillsWorkspaceOverride(t *testing.T) {
	builtinDir := t.TempDir()
	workspaceDir := t.TempDir()

	writePreloadSkill(t, builtinDir, "messaging", "Builtin version.")
	writePreloadSkill(t, workspaceDir, "messaging", "Workspace override.")

	result, err := PreloadedSkills(builtinDir, workspaceDir)
	if err != nil {
		t.Fatalf("PreloadedSkills: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("got %d preloaded skills, want 1 (deduped)", len(result))
	}

	if !strings.Contains(result[0].Content, "Workspace override.") {
		t.Errorf("content = %q, want workspace override", result[0].Content)
	}
}

func TestParseFrontmatterInstall(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "installable")
	os.MkdirAll(skillDir, 0o755)

	content := `---
name: installable
install: true
summary: a skill with install requirements
tools: [create_alarm]
---

# Installable

Body content.
`
	path := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(path, []byte(content), 0o644)

	s, err := parseFrontmatter(path)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}

	if s.Name != "installable" {
		t.Errorf("name = %q, want %q", s.Name, "installable")
	}
}

func TestHandleLoadRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()

	cases := []string{
		"../../../etc",
		"foo/bar",
		"valid\\..\\etc",
		"..%2f..%2fetc",
	}
	for _, name := range cases {
		out, err := handleLoad([]string{dir}, name, nil)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", name, err)
		}
		if !strings.Contains(out, "invalid skill name") {
			t.Fatalf("expected invalid skill name error for %q, got %q", name, out)
		}
	}
}

func TestHandleLoadAcceptsValidName(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "vault")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Vault"), 0o644)

	out, err := handleLoad([]string{dir}, "vault", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "# Vault") {
		t.Fatalf("expected skill content, got %q", out)
	}
}

func TestHandleLoadPreflightWarning(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my_skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: my_skill
summary: needs shell
tools: [shell, write_file]
---

# My Skill
`), 0o644)

	available := func() []string { return []string{"write_file", "db_query"} }
	out, err := handleLoad([]string{dir}, "my_skill", available)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "shell") {
		t.Fatalf("expected warning about missing shell, got %q", out)
	}

	warnLine := strings.SplitN(out, "\n", 2)[0]
	if strings.Contains(warnLine, "write_file") {
		t.Fatalf("warning should not mention write_file (available), got %q", warnLine)
	}
}

func TestHandleLoadNoWarningWhenAllPresent(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "ok_skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: ok_skill
summary: all tools present
tools: [shell]
---

# OK
`), 0o644)

	available := func() []string { return []string{"shell", "db_query"} }
	out, err := handleLoad([]string{dir}, "ok_skill", available)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "warning") {
		t.Fatalf("should not warn when all tools present, got %q", out)
	}
}

func writeSkill(t *testing.T, dir, folder, name, summary, tools string) {
	t.Helper()
	skillDir := filepath.Join(dir, folder)
	os.MkdirAll(skillDir, 0o755)
	content := "---\nname: " + name + "\nsummary: " + summary + "\ntools: " + tools + "\n---\n"
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)
}

func writePreloadSkill(t *testing.T, dir, name, body string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	os.MkdirAll(skillDir, 0o755)
	content := "---\nname: " + name + "\npreload: true\nsummary: preloaded\ntools: [t1]\n---\n\n" + body + "\n"
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)
}
