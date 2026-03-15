package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func writeSkillFile(t *testing.T, dir, name, content string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)
}

func TestSkillChangeReflexDetectsAdded(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	os.MkdirAll(skillsDir, 0o755)

	cfg := &config.Config{Home: dir}

	writeSkillFile(t, skillsDir, "journal", "---\nname: journal\nsummary: daily journal\n---\n# Journal\n")

	ctx := context.Background()
	reflex := SkillChangeReflex(cfg, conn)
	reflex(ctx)

	latest, err := db.SkillEventLatestPerName(ctx, conn)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}

	if len(latest) != 1 {
		t.Fatalf("expected 1 event, got %d", len(latest))
	}
	if latest[0].Name != "journal" || latest[0].Kind != "added" {
		t.Errorf("expected journal added, got %s %s", latest[0].Name, latest[0].Kind)
	}
}

func TestSkillChangeReflexDetectsRemoved(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	os.MkdirAll(skillsDir, 0o755)

	cfg := &config.Config{Home: dir}
	ctx := context.Background()

	writeSkillFile(t, skillsDir, "journal", "---\nname: journal\nsummary: daily journal\n---\n# Journal\n")

	reflex := SkillChangeReflex(cfg, conn)
	reflex(ctx)

	os.RemoveAll(filepath.Join(skillsDir, "journal"))
	reflex(ctx)

	latest, err := db.SkillEventLatestPerName(ctx, conn)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}

	if len(latest) != 1 {
		t.Fatalf("expected 1 event, got %d", len(latest))
	}
	if latest[0].Kind != "removed" {
		t.Errorf("expected removed, got %s", latest[0].Kind)
	}
}

func TestSkillChangeReflexIgnoresPromptOnlyChanges(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	os.MkdirAll(skillsDir, 0o755)

	cfg := &config.Config{Home: dir}
	ctx := context.Background()

	original := "---\nname: journal\nsummary: daily journal\n---\n# Journal\nSome prompt\n"
	writeSkillFile(t, skillsDir, "journal", original)

	reflex := SkillChangeReflex(cfg, conn)
	reflex(ctx)

	updated := "---\nname: journal\nsummary: daily journal\n---\n# Journal\nUpdated prompt\n"
	writeSkillFile(t, skillsDir, "journal", updated)
	reflex(ctx)

	latest, err := db.SkillEventLatestPerName(ctx, conn)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}

	if len(latest) != 1 {
		t.Fatalf("expected 1 event, got %d", len(latest))
	}
	if latest[0].Kind != "added" {
		t.Errorf("expected only added (no changed), got %s", latest[0].Kind)
	}
}

func TestSkillChangeReflexDetectsInstallChange(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	os.MkdirAll(skillsDir, 0o755)

	cfg := &config.Config{Home: dir}
	ctx := context.Background()

	original := "---\nname: journal\nsummary: daily journal\ninstall: true\n---\n# Journal\n\n## Install\nCreate alarm at 9pm\n"
	writeSkillFile(t, skillsDir, "journal", original)

	reflex := SkillChangeReflex(cfg, conn)
	reflex(ctx)

	updated := "---\nname: journal\nsummary: daily journal\ninstall: true\n---\n# Journal\n\n## Install\nCreate alarm at 10pm\n"
	writeSkillFile(t, skillsDir, "journal", updated)
	reflex(ctx)

	latest, err := db.SkillEventLatestPerName(ctx, conn)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}

	if len(latest) != 1 {
		t.Fatalf("expected 1 event, got %d", len(latest))
	}
	if latest[0].Kind != "changed" {
		t.Errorf("expected changed, got %s", latest[0].Kind)
	}
}

func TestSkillChangeReflexIdempotent(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	os.MkdirAll(skillsDir, 0o755)

	cfg := &config.Config{Home: dir}
	ctx := context.Background()

	writeSkillFile(t, skillsDir, "journal", "---\nname: journal\nsummary: daily journal\n---\n# Journal\n")

	reflex := SkillChangeReflex(cfg, conn)
	reflex(ctx)
	reflex(ctx)
	reflex(ctx)

	events, err := db.SkillEventList(ctx, conn, time.Time{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event total, got %d", len(events))
	}
}

func TestSkillChangeReflexRecoversFromDBWipe(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	os.MkdirAll(skillsDir, 0o755)

	cfg := &config.Config{Home: dir}
	ctx := context.Background()

	writeSkillFile(t, skillsDir, "journal", "---\nname: journal\nsummary: daily journal\ninstall: true\n---\n# Journal\n\n## Install\nCreate alarm\n")
	writeSkillFile(t, skillsDir, "briefing", "---\nname: briefing\nsummary: morning briefing\ninstall: true\n---\n# Briefing\n\n## Install\nCreate alarm\n")

	reflex := SkillChangeReflex(cfg, conn)
	reflex(ctx)

	events, err := db.SkillEventList(ctx, conn, time.Time{})
	if err != nil {
		t.Fatalf("list after first run: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events after first run, got %d", len(events))
	}

	_, err = conn.ExecContext(ctx, "DELETE FROM skill_event")
	if err != nil {
		t.Fatalf("wipe skill_event: %v", err)
	}

	reflex(ctx)

	events, err = db.SkillEventList(ctx, conn, time.Time{})
	if err != nil {
		t.Fatalf("list after recovery: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events after DB wipe recovery, got %d", len(events))
	}

	byName := map[string]db.SkillEvent{}
	for _, e := range events {
		byName[e.Name] = e
	}
	if byName["journal"].Kind != "added" {
		t.Errorf("expected journal re-added, got %s", byName["journal"].Kind)
	}
	if byName["briefing"].Kind != "added" {
		t.Errorf("expected briefing re-added, got %s", byName["briefing"].Kind)
	}
}

func TestExtractInstallSection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "no install section",
			content: "---\nname: foo\n---\n# Foo\nSome content\n",
			want:    "",
		},
		{
			name:    "install section at end",
			content: "---\nname: foo\n---\n# Foo\nSome content\n\n## Install\nCreate an alarm\n",
			want:    "## Install\nCreate an alarm",
		},
		{
			name:    "install section with following section",
			content: "---\nname: foo\n---\n# Foo\n\n## Install\nCreate an alarm\n\n## Usage\nUse it\n",
			want:    "## Install\nCreate an alarm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractInstallSection(tt.content)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
