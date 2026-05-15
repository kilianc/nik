package skills

import (
	"bytes"
	"context"
	"database/sql"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func localRunner(_ context.Context, command, stdin string) (string, string, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

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

	err = db.SystemContactEnsure(ctx, conn)
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
		PrivilegedConversationIDs: config.ConversationList{
			{Label: "owner", ID: privilegedConvID},
		},
	}

	h := &reflexHarness{
		conn:      conn,
		cfg:       cfg,
		skillsDir: skillsDir,
		ctx:       ctx,
	}

	return h, SkillChangeReflex(cfg, conn, []fs.FS{os.DirFS(skillsDir)})
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

func TestSkillChangeReflex(t *testing.T) {
	t.Run("detects added", func(t *testing.T) {
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
		if latestSkillEvent(t, h.ctx, h.conn, "journal").Kind != "added" {
			t.Fatalf("expected latest event added")
		}
		if count := countSystemMessages(t, h.ctx, h.conn, "skill_added"); count != 1 {
			t.Fatalf("expected 1 skill_added message, got %d", count)
		}
	})

	t.Run("detects removed", func(t *testing.T) {
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
		if len(skills) != 1 || skills[0].Status != "removed" {
			t.Fatalf("expected removed status, got %v", skills)
		}
		if latestSkillEvent(t, h.ctx, h.conn, "journal").Kind != "removed" {
			t.Fatalf("expected latest event removed")
		}
		if count := countSystemMessages(t, h.ctx, h.conn, "skill_removed"); count != 1 {
			t.Fatalf("expected 1 skill_removed message, got %d", count)
		}
	})

	t.Run("ignores prompt-only changes", func(t *testing.T) {
		h, reflex := setupReflexTest(t)
		writeSkillFile(t, h.skillsDir, "journal", "---\nname: journal\nsummary: daily journal\n---\n# Journal\nSome prompt\n")
		reflex(h.ctx)
		writeSkillFile(t, h.skillsDir, "journal", "---\nname: journal\nsummary: daily journal\n---\n# Journal\nUpdated prompt\n")
		reflex(h.ctx)

		events, err := db.SkillEventList(h.ctx, h.conn, time.Time{})
		if err != nil {
			t.Fatalf("list skill events: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 skill event, got %d", len(events))
		}
		if count := countSystemMessages(t, h.ctx, h.conn, "skill_changed"); count != 0 {
			t.Fatalf("expected 0 skill_changed messages, got %d", count)
		}
	})

	t.Run("detects install change", func(t *testing.T) {
		h, reflex := setupReflexTest(t)
		writeSkillFile(t, h.skillsDir, "journal", "---\nname: journal\nsummary: daily journal\ninstall: true\n---\n# Journal\n\n## Install\nCreate alarm at 9pm\n")
		reflex(h.ctx)

		skillsBefore, err := db.SkillList(h.ctx, h.conn)
		if err != nil {
			t.Fatalf("list skills before: %v", err)
		}
		firstHash := skillsBefore[0].InstallHash.String

		writeSkillFile(t, h.skillsDir, "journal", "---\nname: journal\nsummary: daily journal\ninstall: true\n---\n# Journal\n\n## Install\nCreate alarm at 10pm\n")
		reflex(h.ctx)

		skillsAfter, err := db.SkillList(h.ctx, h.conn)
		if err != nil {
			t.Fatalf("list skills after: %v", err)
		}
		if skillsAfter[0].InstallHash.String == firstHash {
			t.Fatalf("expected install hash to change")
		}
		if latestSkillEvent(t, h.ctx, h.conn, "journal").Kind != "changed" {
			t.Fatalf("expected latest event changed")
		}
		if count := countSystemMessages(t, h.ctx, h.conn, "skill_changed"); count != 1 {
			t.Fatalf("expected 1 skill_changed message, got %d", count)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
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
	})

	t.Run("recovers from db wipe", func(t *testing.T) {
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
		if count := countSystemMessages(t, h.ctx, h.conn, "skill_added"); count != 4 {
			t.Fatalf("expected 4 skill_added messages after recovery, got %d", count)
		}
	})

	t.Run("null install hash when no install section", func(t *testing.T) {
		h, reflex := setupReflexTest(t)
		writeSkillFile(t, h.skillsDir, "shell", "---\nname: shell\nsummary: shell tool\n---\n# Shell\nRun commands\n")
		writeSkillFile(t, h.skillsDir, "journal", "---\nname: journal\nsummary: daily journal\ninstall: true\n---\n# Journal\n\n## Install\nCreate alarm at 11:30pm\n")
		reflex(h.ctx)

		skills, err := db.SkillList(h.ctx, h.conn)
		if err != nil {
			t.Fatalf("list skills: %v", err)
		}
		for _, s := range skills {
			switch s.Name {
			case "shell":
				if s.InstallHash.Valid {
					t.Fatalf("expected NULL install_hash for shell, got %q", s.InstallHash.String)
				}
			case "journal":
				if !s.InstallHash.Valid || s.InstallHash.String == "" {
					t.Fatalf("expected non-empty install_hash for journal")
				}
			}
		}
	})
}

func TestSkillChangeReflexOnlyFirstPrivilegedConv(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ctx := context.Background()

	err = db.SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	convA := "aaa-first"
	convB := "zzz-second"
	for _, id := range []string{convA, convB} {
		_, err = conn.ExecContext(ctx,
			"INSERT INTO conversation (id, platform, external_conversation_id) VALUES (?, 'whatsapp', ?)",
			id, id+"@s.whatsapp.net",
		)
		if err != nil {
			t.Fatalf("seed conversation %s: %v", id, err)
		}
	}

	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	err = os.MkdirAll(skillsDir, 0o755)
	if err != nil {
		t.Fatalf("mkdir skills dir: %v", err)
	}

	cfg := &config.Config{
		Home: dir,
		PrivilegedConversationIDs: config.ConversationList{
			{Label: "owner", ID: convA},
			{Label: "backup", ID: convB},
		},
	}

	reflex := SkillChangeReflex(cfg, conn, []fs.FS{os.DirFS(skillsDir)})
	writeSkillFile(t, skillsDir, "journal", "---\nname: journal\nsummary: daily journal\n---\n# Journal\n")

	reflex(ctx)

	var total int
	err = conn.QueryRowContext(ctx, `SELECT count(*) FROM message WHERE kind = 'skill_added'`).Scan(&total)
	if err != nil {
		t.Fatalf("count total skill_added: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 skill_added message total, got %d", total)
	}

	var targetConv string
	err = conn.QueryRowContext(ctx, `SELECT conversation_id FROM message WHERE kind = 'skill_added'`).Scan(&targetConv)
	if err != nil {
		t.Fatalf("get target conversation: %v", err)
	}
	if targetConv != convA {
		t.Fatalf("expected skill_added in %s, got %s", convA, targetConv)
	}
}

func mockCompleter(cronExpr string) Completer {
	return func(_ context.Context, _, _ string) (string, error) {
		return cronExpr, nil
	}
}

func TestSkillCheckReflex(t *testing.T) {
	t.Run("new meta from check", func(t *testing.T) {
		h, _ := setupReflexTest(t)

		def := SkillReflexDef{
			Name:    "check",
			Command: `echo '{"new":"data"}'`,
			Every:   "every minute",
		}

		runSkillCheck(h.ctx, h.cfg, h.conn, "test_skill/check", def, "", localRunner)

		meta, _, err := db.SkillReflexLatest(h.ctx, h.conn, "test_skill/check")
		if err != nil {
			t.Fatalf("get latest: %v", err)
		}
		if meta != `{"new":"data"}` {
			t.Fatalf("expected meta, got %q", meta)
		}
	})

	t.Run("no output", func(t *testing.T) {
		h, _ := setupReflexTest(t)

		def := SkillReflexDef{
			Name:    "check",
			Command: "true",
			Every:   "every minute",
		}

		runSkillCheck(h.ctx, h.cfg, h.conn, "silent_skill/check", def, "", localRunner)

		meta, _, err := db.SkillReflexLatest(h.ctx, h.conn, "silent_skill/check")
		if err != nil {
			t.Fatalf("get latest: %v", err)
		}
		if meta != "" {
			t.Fatalf("expected empty meta, got %q", meta)
		}
	})

	t.Run("same meta skipped", func(t *testing.T) {
		h, _ := setupReflexTest(t)

		err := db.SkillReflexInsert(h.ctx, h.conn, "stable_skill/check", "same-meta")
		if err != nil {
			t.Fatalf("seed meta: %v", err)
		}

		def := SkillReflexDef{
			Name:    "check",
			Command: "echo 'same-meta'",
			Every:   "every minute",
		}

		runSkillCheck(h.ctx, h.cfg, h.conn, "stable_skill/check", def, "same-meta", localRunner)

		var count int
		err = h.conn.QueryRowContext(h.ctx, "SELECT count(*) FROM skill_reflex WHERE skill_name = 'stable_skill/check'").Scan(&count)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected 1 row (no new insert), got %d", count)
		}
	})

	t.Run("schedule-only fires with timestamp", func(t *testing.T) {
		h, _ := setupReflexTest(t)

		def := SkillReflexDef{
			Name:  "journal",
			Every: "every day at 6am",
		}

		runSkillCheck(h.ctx, h.cfg, h.conn, "journal/journal", def, "", localRunner)

		meta, _, err := db.SkillReflexLatest(h.ctx, h.conn, "journal/journal")
		if err != nil {
			t.Fatalf("get latest: %v", err)
		}
		if meta == "" {
			t.Fatal("expected non-empty meta for schedule-only reflex")
		}

		if count := countSystemMessages(t, h.ctx, h.conn, "skill_reflex_fired"); count != 1 {
			t.Fatalf("expected 1 skill_reflex_fired message, got %d", count)
		}
	})

	t.Run("skips not-due reflexes on restart", func(t *testing.T) {
		h, _ := setupReflexTest(t)

		writeSkillFile(t, h.skillsDir, "journal", "---\nname: journal\nsummary: daily journal\nreflex:\n  - name: journal\n    every: \"every day at 11pm\"\n---\n# Journal\n")

		err := db.SkillReflexInsert(h.ctx, h.conn, "journal/journal", "2026-03-25T23:00:00Z")
		if err != nil {
			t.Fatalf("seed reflex: %v", err)
		}

		checkReflex := SkillCheckReflex(h.cfg, h.conn, mockCompleter("0 23 * * *"), localRunner, []fs.FS{os.DirFS(h.skillsDir)})
		checkReflex(h.ctx)

		var count int
		err = h.conn.QueryRowContext(h.ctx,
			"SELECT count(*) FROM skill_reflex WHERE skill_name = 'journal/journal'",
		).Scan(&count)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected 1 row (not due yet), got %d", count)
		}

		if msgCount := countSystemMessages(t, h.ctx, h.conn, "skill_reflex_fired"); msgCount != 0 {
			t.Fatalf("expected 0 skill_reflex_fired messages (not due), got %d", msgCount)
		}
	})

	t.Run("skips new reflex not yet due today", func(t *testing.T) {
		h, _ := setupReflexTest(t)

		writeSkillFile(t, h.skillsDir, "journal", "---\nname: journal\nsummary: daily journal\nreflex:\n  - name: journal\n    every: \"every day at 11pm\"\n---\n# Journal\n")

		checkReflex := SkillCheckReflex(h.cfg, h.conn, mockCompleter("0 23 * * *"), localRunner, []fs.FS{os.DirFS(h.skillsDir)})
		checkReflex(h.ctx)

		var count int
		err := h.conn.QueryRowContext(h.ctx,
			"SELECT count(*) FROM skill_reflex WHERE skill_name = 'journal/journal'",
		).Scan(&count)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected 0 rows (new reflex, not due yet), got %d", count)
		}

		if msgCount := countSystemMessages(t, h.ctx, h.conn, "skill_reflex_fired"); msgCount != 0 {
			t.Fatalf("expected 0 skill_reflex_fired messages, got %d", msgCount)
		}
	})
}

func TestRunSkillCheckUsesCommandRunner(t *testing.T) {
	h, _ := setupReflexTest(t)

	var gotCommand, gotStdin string
	runner := func(_ context.Context, command, stdin string) (string, string, error) {
		gotCommand = command
		gotStdin = stdin
		return `{"from":"runner"}`, "", nil
	}

	def := SkillReflexDef{
		Name:    "check",
		Command: "sh skills/test/check.sh",
		Every:   "every minute",
	}

	runSkillCheck(h.ctx, h.cfg, h.conn, "test_skill/check", def, "prev-meta", runner)

	if gotCommand != "sh skills/test/check.sh" {
		t.Fatalf("expected runner to receive command, got %q", gotCommand)
	}
	if gotStdin != "prev-meta" {
		t.Fatalf("expected runner to receive stdin, got %q", gotStdin)
	}

	meta, _, err := db.SkillReflexLatest(h.ctx, h.conn, "test_skill/check")
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if meta != `{"from":"runner"}` {
		t.Fatalf("expected meta from runner, got %q", meta)
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
