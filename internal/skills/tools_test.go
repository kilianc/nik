package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantName    string
		wantSummary string
		wantTools   []string
		wantPreload bool
	}{
		{
			name:        "inline tools",
			content:     "---\nname: test_skill\nsummary: >\n  Load this skill to test frontmatter parsing.\ntools: [tool_a, tool_b]\n---\n\n# Test Skill\n\nSome content here.\n",
			wantName:    "test_skill",
			wantSummary: "Load this skill to test frontmatter parsing.",
			wantTools:   []string{"tool_a", "tool_b"},
		},
		{
			name:      "block tools",
			content:   "---\nname: block\nsummary: block test\ntools:\n  - alpha\n  - beta\n  - gamma\n---\n\n# Block\n",
			wantName:  "block",
			wantTools: []string{"alpha", "beta", "gamma"},
		},
		{
			name:        "preload true",
			content:     "---\nname: preloaded\npreload: true\nsummary: a preloaded skill\ntools: [tool_x]\n---\n\n# Preloaded\n\nBody content.\n",
			wantName:    "preloaded",
			wantPreload: true,
		},
		{
			name:     "preload default false",
			content:  "---\nname: normal\nsummary: no preload field\ntools: [t1]\n---\n\n# Normal\n",
			wantName: "normal",
		},
		{
			name:     "install field",
			content:  "---\nname: installable\ninstall: true\nsummary: a skill with install requirements\ntools: [create_alarm]\n---\n\n# Installable\n\nBody content.\n",
			wantName: "installable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := parseFrontmatter([]byte(tt.content))
			if err != nil {
				t.Fatalf("parseFrontmatter: %v", err)
			}

			if s.Name != tt.wantName {
				t.Errorf("name = %q, want %q", s.Name, tt.wantName)
			}
			if tt.wantSummary != "" && s.Summary != tt.wantSummary {
				t.Errorf("summary = %q, want %q", s.Summary, tt.wantSummary)
			}
			if tt.wantTools != nil {
				if len(s.Tools) != len(tt.wantTools) {
					t.Fatalf("tools len = %d, want %d", len(s.Tools), len(tt.wantTools))
				}
				for i, w := range tt.wantTools {
					if s.Tools[i] != w {
						t.Errorf("tools[%d] = %q, want %q", i, s.Tools[i], w)
					}
				}
			}
			if s.Preload != tt.wantPreload {
				t.Errorf("preload = %v, want %v", s.Preload, tt.wantPreload)
			}
		})
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

	if len(result[0].Tools) != 1 || result[0].Tools[0] != "t1" {
		t.Errorf("tools = %v, want [t1]", result[0].Tools)
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

func TestHandleLoadRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()

	cases := []string{
		"../../../etc",
		"foo/bar",
		"valid\\..\\etc",
	}
	for _, name := range cases {
		out, err := handleLoad([]string{dir}, name, nil)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", name, err)
		}
		if !strings.Contains(out, "not found") {
			t.Fatalf("expected not found error for %q, got %q", name, out)
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
	t.Run("warns about missing tools", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, "my_skill")
		os.MkdirAll(skillDir, 0o755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my_skill\nsummary: needs shell\ntools: [shell, write_file]\n---\n\n# My Skill\n"), 0o644)

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
	})

	t.Run("no warning when all present", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, "ok_skill")
		os.MkdirAll(skillDir, 0o755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: ok_skill\nsummary: all tools present\ntools: [shell]\n---\n\n# OK\n"), 0o644)

		available := func() []string { return []string{"shell", "db_query"} }
		out, err := handleLoad([]string{dir}, "ok_skill", available)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "warning") {
			t.Fatalf("should not warn when all tools present, got %q", out)
		}
	})
}

func TestSymlinkEscapeBlocked(t *testing.T) {
	t.Run("handleLoad", func(t *testing.T) {
		dir := t.TempDir()
		outside := t.TempDir()

		skillDir := filepath.Join(outside, "secret")
		os.MkdirAll(skillDir, 0o755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Secret"), 0o644)

		err := os.Symlink(outside, filepath.Join(dir, "escape"))
		if err != nil {
			t.Fatal(err)
		}

		out, loadErr := handleLoad([]string{dir}, "escape/secret", nil)
		if loadErr != nil {
			t.Fatalf("unexpected error: %v", loadErr)
		}
		if !strings.Contains(out, "not found") {
			t.Fatalf("expected not found for symlink escape, got %q", out)
		}
	})

	t.Run("walkSkillDirs", func(t *testing.T) {
		dir := t.TempDir()
		outside := t.TempDir()

		skillDir := filepath.Join(outside, "secret")
		os.MkdirAll(skillDir, 0o755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: secret\nsummary: escaped\n---\n"), 0o644)

		err := os.Symlink(skillDir, filepath.Join(dir, "escape"))
		if err != nil {
			t.Fatal(err)
		}

		var found []string
		walkErr := walkSkillDirs([]string{dir}, func(s SkillSummary, _ []byte) {
			found = append(found, s.Name)
		})
		if walkErr != nil {
			t.Fatalf("walk error: %v", walkErr)
		}

		for _, name := range found {
			if name == "secret" {
				t.Fatalf("symlink escape should have been blocked, but found skill %q", name)
			}
		}
	})
}

func TestStripInstallSection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no install section",
			input: "# Skill\n\nBody content.\n",
			want:  "# Skill\n\nBody content.\n",
		},
		{
			name:  "install at end",
			input: "# Skill\n\nBody content.\n\n## Install\n\nCreate an alarm.\n",
			want:  "# Skill\n\nBody content.\n",
		},
		{
			name:  "install in middle",
			input: "# Skill\n\nBody.\n\n## Install\n\nCreate an alarm.\n\n## Behavior\n\nDo things.\n",
			want:  "# Skill\n\nBody.\n\n## Behavior\n\nDo things.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripInstallSection(tt.input)
			if got != tt.want {
				t.Errorf("stripInstallSection =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestPreloadedSkillsStripsInstall(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, "myskill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: myskill
preload: true
summary: a skill
tools: [t1]
---

# My Skill

Body content.

## Install

Create a recurring alarm.

## Behavior

Do things.
`), 0o644)

	result, err := PreloadedSkills(dir)
	if err != nil {
		t.Fatalf("PreloadedSkills: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("got %d preloaded skills, want 1", len(result))
	}

	if strings.Contains(result[0].Content, "## Install") {
		t.Errorf("content should not contain install section, got: %q", result[0].Content)
	}

	if !strings.Contains(result[0].Content, "Body content.") {
		t.Errorf("content missing body, got: %q", result[0].Content)
	}

	if !strings.Contains(result[0].Content, "## Behavior") {
		t.Errorf("content missing behavior section, got: %q", result[0].Content)
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
