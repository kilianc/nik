package skills

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

const privilegedConvID = "priv-conv-001"

type reflexHarness struct {
	conn      *sql.DB
	cfg       *config.Config
	skillsDir string
	ctx       context.Context
}

func writeSkillFile(t *testing.T, dir, name, content string) {
	t.Helper()

	skillDir := filepath.Join(dir, name)
	err := os.MkdirAll(skillDir, 0o755)
	if err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}

	err = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)
	if err != nil {
		t.Fatalf("write skill file: %v", err)
	}
}

func setupReflexTest(t *testing.T) (*reflexHarness, func(context.Context)) {
	t.Helper()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ctx := context.Background()

	err = db.EnsureSystemContact(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	_, err = conn.ExecContext(ctx,
		"INSERT INTO conversation (id, platform, external_conversation_id) VALUES (?, 'whatsapp', 'owner@s.whatsapp.net')",
		privilegedConvID,
	)
	if err != nil {
		t.Fatalf("seed privileged conversation: %v", err)
	}

	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	err = os.MkdirAll(skillsDir, 0o755)
	if err != nil {
		t.Fatalf("mkdir skills dir: %v", err)
	}

	cfg := &config.Config{
		Home: dir,
		PrivilegedConversationIDs: map[string]string{
			"owner": privilegedConvID,
		},
	}

	h := &reflexHarness{
		conn:      conn,
		cfg:       cfg,
		skillsDir: skillsDir,
		ctx:       ctx,
	}

	return h, SkillChangeReflex(cfg, conn)
}

func latestSkillEvent(t *testing.T, ctx context.Context, conn *sql.DB, name string) db.SkillEvent {
	t.Helper()

	events, err := db.SkillEventLatestPerName(ctx, conn)
	if err != nil {
		t.Fatalf("latest skill events: %v", err)
	}

	for _, e := range events {
		if e.Name == name {
			return e
		}
	}

	t.Fatalf("missing latest skill event for %s", name)
	return db.SkillEvent{}
}

func countSystemMessages(t *testing.T, ctx context.Context, conn *sql.DB, kind string) int {
	t.Helper()

	var count int
	err := conn.QueryRowContext(ctx,
		`SELECT count(*) FROM message WHERE conversation_id = ?1 AND kind = ?2`,
		privilegedConvID,
		kind,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count system messages: %v", err)
	}

	return count
}

func TestSkillChangeReflexDetectsAdded(t *testing.T) {
	h, reflex := setupReflexTest(t)

	writeSkillFile(t, h.skillsDir, "journal", "---\nname: journal\nsummary: daily journal\n---\n# Journal\n")

	reflex(h.ctx)

	skills, err := db.SkillList(h.ctx, h.conn)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "journal" || skills[0].Status != "active" {
		t.Fatalf("expected journal active, got %s %s", skills[0].Name, skills[0].Status)
	}

	event := latestSkillEvent(t, h.ctx, h.conn, "journal")
	if event.Kind != "added" {
		t.Fatalf("expected latest event added, got %s", event.Kind)
	}

	if count := countSystemMessages(t, h.ctx, h.conn, "skill_added"); count != 1 {
		t.Fatalf("expected 1 skill_added message, got %d", count)
	}
}

func TestSkillChangeReflexDetectsRemoved(t *testing.T) {
	h, reflex := setupReflexTest(t)

	writeSkillFile(t, h.skillsDir, "journal", "---\nname: journal\nsummary: daily journal\n---\n# Journal\n")
	reflex(h.ctx)

	err := os.RemoveAll(filepath.Join(h.skillsDir, "journal"))
	if err != nil {
		t.Fatalf("remove skill dir: %v", err)
	}

	reflex(h.ctx)

	skills, err := db.SkillList(h.ctx, h.conn)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill row, got %d", len(skills))
	}
	if skills[0].Status != "removed" {
		t.Fatalf("expected removed status, got %s", skills[0].Status)
	}

	event := latestSkillEvent(t, h.ctx, h.conn, "journal")
	if event.Kind != "removed" {
		t.Fatalf("expected latest event removed, got %s", event.Kind)
	}

	if count := countSystemMessages(t, h.ctx, h.conn, "skill_removed"); count != 1 {
		t.Fatalf("expected 1 skill_removed message, got %d", count)
	}
}

func TestSkillChangeReflexIgnoresPromptOnlyChanges(t *testing.T) {
	h, reflex := setupReflexTest(t)

	original := "---\nname: journal\nsummary: daily journal\n---\n# Journal\nSome prompt\n"
	writeSkillFile(t, h.skillsDir, "journal", original)
	reflex(h.ctx)

	updated := "---\nname: journal\nsummary: daily journal\n---\n# Journal\nUpdated prompt\n"
	writeSkillFile(t, h.skillsDir, "journal", updated)
	reflex(h.ctx)

	events, err := db.SkillEventList(h.ctx, h.conn, time.Time{})
	if err != nil {
		t.Fatalf("list skill events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 skill event, got %d", len(events))
	}

	if count := countSystemMessages(t, h.ctx, h.conn, "skill_added"); count != 1 {
		t.Fatalf("expected only initial skill_added message, got %d", count)
	}
	if count := countSystemMessages(t, h.ctx, h.conn, "skill_changed"); count != 0 {
		t.Fatalf("expected 0 skill_changed messages, got %d", count)
	}
}

func TestSkillChangeReflexDetectsInstallChange(t *testing.T) {
	h, reflex := setupReflexTest(t)

	original := "---\nname: journal\nsummary: daily journal\ninstall: true\n---\n# Journal\n\n## Install\nCreate alarm at 9pm\n"
	writeSkillFile(t, h.skillsDir, "journal", original)
	reflex(h.ctx)

	skillsBefore, err := db.SkillList(h.ctx, h.conn)
	if err != nil {
		t.Fatalf("list skills before: %v", err)
	}
	firstHash := skillsBefore[0].InstallHash.String

	updated := "---\nname: journal\nsummary: daily journal\ninstall: true\n---\n# Journal\n\n## Install\nCreate alarm at 10pm\n"
	writeSkillFile(t, h.skillsDir, "journal", updated)
	reflex(h.ctx)

	skillsAfter, err := db.SkillList(h.ctx, h.conn)
	if err != nil {
		t.Fatalf("list skills after: %v", err)
	}
	if len(skillsAfter) != 1 {
		t.Fatalf("expected 1 skill row, got %d", len(skillsAfter))
	}
	if skillsAfter[0].InstallHash.String == firstHash {
		t.Fatalf("expected install hash to change")
	}

	event := latestSkillEvent(t, h.ctx, h.conn, "journal")
	if event.Kind != "changed" {
		t.Fatalf("expected latest event changed, got %s", event.Kind)
	}

	if count := countSystemMessages(t, h.ctx, h.conn, "skill_changed"); count != 1 {
		t.Fatalf("expected 1 skill_changed message, got %d", count)
	}
}

func TestSkillChangeReflexIdempotent(t *testing.T) {
	h, reflex := setupReflexTest(t)

	writeSkillFile(t, h.skillsDir, "journal", "---\nname: journal\nsummary: daily journal\n---\n# Journal\n")

	reflex(h.ctx)
	reflex(h.ctx)
	reflex(h.ctx)

	events, err := db.SkillEventList(h.ctx, h.conn, time.Time{})
	if err != nil {
		t.Fatalf("list skill events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 skill event total, got %d", len(events))
	}

	if count := countSystemMessages(t, h.ctx, h.conn, "skill_added"); count != 1 {
		t.Fatalf("expected 1 skill_added message, got %d", count)
	}
}

func TestSkillChangeReflexRecoversFromDBWipe(t *testing.T) {
	h, reflex := setupReflexTest(t)

	writeSkillFile(t, h.skillsDir, "journal", "---\nname: journal\nsummary: daily journal\ninstall: true\n---\n# Journal\n\n## Install\nCreate alarm\n")
	writeSkillFile(t, h.skillsDir, "briefing", "---\nname: briefing\nsummary: morning briefing\ninstall: true\n---\n# Briefing\n\n## Install\nCreate alarm\n")

	reflex(h.ctx)

	_, err := h.conn.ExecContext(h.ctx, "DELETE FROM skill")
	if err != nil {
		t.Fatalf("wipe skill table: %v", err)
	}

	reflex(h.ctx)

	skills, err := db.SkillList(h.ctx, h.conn)
	if err != nil {
		t.Fatalf("list skills after recovery: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills after recovery, got %d", len(skills))
	}

	if latestSkillEvent(t, h.ctx, h.conn, "journal").Kind != "added" {
		t.Fatalf("expected journal to be re-added")
	}
	if latestSkillEvent(t, h.ctx, h.conn, "briefing").Kind != "added" {
		t.Fatalf("expected briefing to be re-added")
	}

	if count := countSystemMessages(t, h.ctx, h.conn, "skill_added"); count != 4 {
		t.Fatalf("expected 4 skill_added messages after recovery, got %d", count)
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
