package timeline

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func skillMessage(kind string, skill db.Skill) db.Message {
	body, _ := json.Marshal(skill)
	return db.Message{
		Platform: "system",
		Kind:     kind,
		Body:     string(body),
		SentAt:   time.Now(),
	}
}

func TestRenderSkillEventAddedWithInstall(t *testing.T) {
	msg := skillMessage("skill_added", db.Skill{
		Name:        "breathing",
		InstallHash: sql.NullString{String: "abc123", Valid: true},
	})

	e := renderSkillEvent(msg)

	if !strings.Contains(e.text, "MANDATORY") {
		t.Fatalf("expected MANDATORY directive, got: %s", e.text)
	}
	if !strings.Contains(e.text, "load_skill") {
		t.Fatalf("expected load_skill instruction, got: %s", e.text)
	}
	if !strings.Contains(e.text, "[Skill added]") {
		t.Fatalf("expected [Skill added] header, got: %s", e.text)
	}
}

func TestRenderSkillEventAddedWithoutInstall(t *testing.T) {
	msg := skillMessage("skill_added", db.Skill{
		Name: "shell",
	})

	e := renderSkillEvent(msg)

	if strings.Contains(e.text, "MANDATORY") {
		t.Fatalf("expected no MANDATORY directive for skill without install, got: %s", e.text)
	}
	if !strings.Contains(e.text, "[Skill added]") {
		t.Fatalf("expected [Skill added] header, got: %s", e.text)
	}
	if !strings.Contains(e.text, "shell") {
		t.Fatalf("expected skill name in output, got: %s", e.text)
	}
}

func TestRenderSkillEventChangedWithInstall(t *testing.T) {
	msg := skillMessage("skill_changed", db.Skill{
		Name:        "journal",
		InstallHash: sql.NullString{String: "def456", Valid: true},
	})

	e := renderSkillEvent(msg)

	if !strings.Contains(e.text, "MANDATORY") {
		t.Fatalf("expected MANDATORY directive, got: %s", e.text)
	}
	if !strings.Contains(e.text, "idempotently") {
		t.Fatalf("expected idempotent language, got: %s", e.text)
	}
	if !strings.Contains(e.text, "no duplicate alarms") {
		t.Fatalf("expected duplicate alarm warning, got: %s", e.text)
	}
}

func TestRenderSkillEventChangedWithoutInstall(t *testing.T) {
	msg := skillMessage("skill_changed", db.Skill{
		Name: "config",
	})

	e := renderSkillEvent(msg)

	if strings.Contains(e.text, "MANDATORY") {
		t.Fatalf("expected no MANDATORY directive for skill without install, got: %s", e.text)
	}
}

func TestRenderSkillEventRemoved(t *testing.T) {
	msg := skillMessage("skill_removed", db.Skill{
		Name: "journal",
	})

	e := renderSkillEvent(msg)

	if !strings.Contains(e.text, "[Skill removed]") {
		t.Fatalf("expected [Skill removed] header, got: %s", e.text)
	}
	if !strings.Contains(e.text, "ask user") {
		t.Fatalf("expected user confirmation directive, got: %s", e.text)
	}
}
